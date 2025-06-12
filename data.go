// Package main provides Guide2Go, a tool to generate XMLTV files from Schedules Direct JSON API.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
)

const (
	maxConcurrentRequests = 5
	batchSize            = 5000
	metadataBatchSize    = 500
)

var (
	// requestLimiter limits concurrent requests to Schedules Direct API
	requestLimiter = rate.NewLimiter(rate.Every(100*time.Millisecond), maxConcurrentRequests)
)

// Update updates data from Schedules Direct and creates the XMLTV file
func (app *App) Update(ctx context.Context, sd *SD, filename string) error {
	app.Logger.WithField("filename", filename).Info("Starting data update")
	app.Config.File = strings.TrimSuffix(filename, filepath.Ext(filename))
	if _, err := os.ReadFile(fmt.Sprintf("%s.yaml", app.Config.File)); err != nil {
		app.Logger.WithError(err).Error("Failed to read configuration file")
		return errors.Wrap(err, "failed to read configuration file")
	}
	if err := app.Config.Open(ctx); err != nil {
		app.Logger.WithError(err).Error("Failed to open configuration")
		return errors.Wrap(err, "failed to open configuration")
	}
	if err := sd.Init(); err != nil {
		app.Logger.WithError(err).Error("Failed to initialize SD client")
		return errors.Wrap(err, "failed to initialize SD client")
	}
	if len(sd.Token) == 0 {
		if err := sd.Login(); err != nil {
			app.Logger.WithError(err).Error("Failed to login to Schedules Direct")
			return errors.Wrap(err, "failed to login to Schedules Direct")
		}
	}
	if err := sd.GetData(ctx); err != nil {
		app.Logger.WithError(err).Error("Failed to get data from Schedules Direct")
		return errors.Wrap(err, "failed to get data from Schedules Direct")
	}
	runtime.GC()
	if err := app.CreateXMLTV(ctx, filename); err != nil {
		app.Logger.WithError(err).Error("Failed to create XMLTV file")
		return errors.Wrap(err, "failed to create XMLTV file")
	}
	Cache.CleanUp()
	runtime.GC()
	return nil
}

// GetData fetches and processes data from Schedules Direct
func (sd *SD) GetData(ctx context.Context) error {
	logger := logger.WithField("operation", "GetData")

	// Open and initialize cache
	if err := Cache.Open(); err != nil {
		return errors.Wrap(err, "failed to open cache")
	}
	Cache.Init()

	// Get account status
	if err := sd.Status(); err != nil {
		return errors.Wrap(err, "failed to get account status")
	}

	// Process lineups
	if err := sd.processLineups(ctx); err != nil {
		return errors.Wrap(err, "failed to process lineups")
	}

	// Process schedules
	if err := sd.processSchedules(ctx); err != nil {
		return errors.Wrap(err, "failed to process schedules")
	}

	// Process programs and metadata
	if err := sd.processProgramsAndMetadata(ctx); err != nil {
		return errors.Wrap(err, "failed to process programs and metadata")
	}

	// Save cache
	if err := Cache.Save(); err != nil {
		return errors.Wrap(err, "failed to save cache")
	}

	return nil
}

// processLineups processes all lineups from Schedules Direct
func (sd *SD) processLineups(ctx context.Context) error {
	logger := logger.WithField("operation", "processLineups")

	// Reset channel cache
	Cache.Channel = make(map[string]G2GCache)

	// Get lineups from status
	var lineups []string
	for _, l := range sd.Resp.Status.Lineups {
		lineups = append(lineups, l.Lineup)
	}

	// Process each lineup
	for _, id := range lineups {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			sd.Req.Parameter = fmt.Sprintf("/%s", id)
			sd.Req.Type = "GET"

			if err := sd.Lineups(); err != nil {
				logger.WithError(err).WithField("lineup", id).Error("Failed to get lineup")
				continue
			}

			if err := Cache.AddStations(ctx, &sd.Resp.Body, id); err != nil {
				logger.WithError(err).WithField("lineup", id).Error("Failed to add stations")
				continue
			}
		}
	}

	return nil
}

// processSchedules processes schedules for all channels
func (sd *SD) processSchedules(ctx context.Context) error {
	logger := logger.WithField("operation", "processSchedules")

	// Prepare schedule dates
	days := make([]string, Config.Options.Schedule)
	for i := 0; i < Config.Options.Schedule; i++ {
		days[i] = time.Now().Add(time.Hour * time.Duration(24*i)).Format("2006-01-02")
	}

	logger.WithField("days", Config.Options.Schedule).Info("Downloading schedules")

	// Process channels in batches
	var wg sync.WaitGroup
	errChan := make(chan error, 1)

	for i := 0; i < len(Config.Station); i += batchSize {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			end := i + batchSize
			if end > len(Config.Station) {
				end = len(Config.Station)
			}

			// Prepare batch
			channels := make([]interface{}, 0, end-i)
			for _, channel := range Config.Station[i:end] {
				channel.Date = days
				channels = append(channels, channel)
			}

			// Marshal batch data
			data, err := json.Marshal(channels)
			if err != nil {
				return errors.Wrap(err, "failed to marshal channel data")
			}
			sd.Req.Data = data

			// Get schedule data
			if err := sd.Schedule(); err != nil {
				logger.WithError(err).WithField("batch", i/batchSize).Error("Failed to get schedule")
				continue
			}

			// Process schedule data
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := Cache.AddSchedule(ctx, &sd.Resp.Body); err != nil {
					select {
					case errChan <- errors.Wrap(err, "failed to add schedule"):
					default:
					}
				}
			}()
		}
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errChan)

	// Check for errors
	if err := <-errChan; err != nil {
		return err
	}

	return nil
}

// processProgramsAndMetadata processes programs and metadata
func (sd *SD) processProgramsAndMetadata(ctx context.Context) error {
	logger := logger.WithField("operation", "processProgramsAndMetadata")

	// Get program IDs
	programIDs := Cache.GetRequiredProgramIDs()
	allIDs := Cache.GetAllProgramIDs()

	logger.WithFields(logrus.Fields{
		"new":     len(programIDs),
		"cached":  len(allIDs) - len(programIDs),
		"total":   len(allIDs),
	}).Info("Processing programs and metadata")

	// Process programs and metadata
	types := []string{"programs", "metadata"}
	for _, t := range types {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Configure request based on type
			switch t {
			case "metadata":
				sd.Req.URL = fmt.Sprintf("%smetadata/programs", sd.BaseURL)
				sd.Req.Call = "metadata"
				programIDs = Cache.GetRequiredMetaIDs()
				logger.WithField("count", len(programIDs)).Info("Downloading metadata")
			case "programs":
				sd.Req.URL = fmt.Sprintf("%sprograms", sd.BaseURL)
				sd.Req.Call = "programs"
				logger.WithField("count", len(programIDs)).Info("Downloading programs")
			}

			// Process in batches
			batchSize := metadataBatchSize
			if t == "programs" {
				batchSize = batchSize
			}

			var wg sync.WaitGroup
			errChan := make(chan error, 1)

			for i := 0; i < len(programIDs); i += batchSize {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
					end := i + batchSize
					if end > len(programIDs) {
						end = len(programIDs)
					}

					// Prepare batch
					programs := make([]interface{}, 0, end-i)
					for _, p := range programIDs[i:end] {
						programs = append(programs, p)
					}

					// Marshal batch data
					data, err := json.Marshal(programs)
					if err != nil {
						return errors.Wrap(err, "failed to marshal program data")
					}
					sd.Req.Data = data

					// Get program data
					if err := sd.Program(); err != nil {
						logger.WithError(err).WithField("batch", i/batchSize).Error("Failed to get programs")
						continue
					}

					// Process program data
					wg.Add(1)
					go func() {
						defer wg.Done()
						var err error
						switch t {
						case "metadata":
							err = Cache.AddMetadata(ctx, &sd.Resp.Body, &wg)
						case "programs":
							err = Cache.AddProgram(ctx, &sd.Resp.Body, &wg)
						}
						if err != nil {
							select {
							case errChan <- errors.Wrap(err, "failed to add program data"):
							default:
							}
						}
					}()
				}
			}

			// Wait for all goroutines to complete
			wg.Wait()
			close(errChan)

			// Check for errors
			if err := <-errChan; err != nil {
				return err
			}
		}
	}

	return nil
}
