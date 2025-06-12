// Package main provides Guide2Go, a tool to generate XMLTV files from Schedules Direct JSON API.
package main

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"regexp"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Pre-compile the regexp for SanitizeID
var sanitizeIDRegexp = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// XMLTVGenerator represents an XMLTV file generator
type XMLTVGenerator struct {
	encoder *xml.Encoder
	buffer  *bytes.Buffer
	logger  *logrus.Entry
}

// NewXMLTVGenerator creates a new XMLTV generator
func NewXMLTVGenerator() *XMLTVGenerator {
	buf := &bytes.Buffer{}
	buf.WriteString(xml.Header)

	enc := xml.NewEncoder(buf)
	enc.Indent("", "  ")

	return &XMLTVGenerator{
		encoder: enc,
		buffer:  buf,
		logger:  logger.WithField("component", "xmltv_generator"),
	}
}

// CreateXMLTV generates the XMLTV file using the provided app context
func (app *App) CreateXMLTV(ctx context.Context, filename string) error {
	app.Logger.WithField("filename", filename).Info("Starting XMLTV creation")
	gen := NewXMLTVGenerator()
	app.Config.File = strings.TrimSuffix(filename, filepath.Ext(filename))
	if err := app.Config.Open(ctx); err != nil {
		app.Logger.WithError(err).Error("Failed to open configuration")
		return errors.Wrap(err, "failed to open configuration")
	}
	if err := Cache.Open(); err != nil {
		app.Logger.WithError(err).Error("Failed to open cache")
		return errors.Wrap(err, "failed to open cache")
	}
	Cache.Init()
	app.Logger.WithField("path", app.Config.Files.XMLTV).Info("Creating XMLTV file")
	if err := gen.writeHeader(); err != nil {
		return errors.Wrap(err, "failed to write XML header")
	}
	if err := gen.writeChannels(ctx); err != nil {
		return errors.Wrap(err, "failed to write channels")
	}
	if err := gen.writePrograms(ctx); err != nil {
		return errors.Wrap(err, "failed to write programs")
	}
	if err := gen.writeFooter(); err != nil {
		return errors.Wrap(err, "failed to write XML footer")
	}
	if err := gen.writeFile(); err != nil {
		return errors.Wrap(err, "failed to write XMLTV file")
	}
	runtime.GC()
	return nil
}

// writeHeader writes the XML header and root element
func (g *XMLTVGenerator) writeHeader() error {
	attrs := []xml.Attr{
		{Name: xml.Name{Local: AppName}, Value: AppName},
		{Name: xml.Name{Local: "source-info-name"}, Value: "Schedules Direct"},
		{Name: xml.Name{Local: "source-info-url"}, Value: "http://schedulesdirect.org"},
	}

	return g.encoder.EncodeToken(xml.StartElement{
		Name: xml.Name{Local: "tv"},
		Attr: attrs,
	})
}

// writeChannels writes all channels to the XML file
func (g *XMLTVGenerator) writeChannels(ctx context.Context) error {
	for _, cache := range Cache.Channel {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			channel := ChannelXML{
				ID: SanitizeID(cache.Callsign),
				Icon: Icon{
					Src:    cache.Logo.URL,
					Height: cache.Logo.Height,
					Width:  cache.Logo.Width,
				},
				DisplayName: []DisplayName{
					{Value: cache.Callsign},
					{Value: cache.Name},
				},
			}

			if err := g.encoder.Encode(channel); err != nil {
				return errors.Wrap(err, "failed to encode channel")
			}
		}
	}

	return nil
}

// writePrograms writes all programs to the XML file
func (g *XMLTVGenerator) writePrograms(ctx context.Context) error {
	for _, cache := range Cache.Channel {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			programs, err := g.getPrograms(cache)
			if err != nil {
				g.logger.WithError(err).WithField("channel", cache.Callsign).Error("Failed to get programs")
				continue
			}

			for _, program := range programs {
				if err := g.encoder.Encode(program); err != nil {
					return errors.Wrap(err, "failed to encode program")
				}
			}
		}
	}

	return nil
}

// writeFooter writes the XML footer
func (g *XMLTVGenerator) writeFooter() error {
	if err := g.encoder.EncodeToken(xml.EndElement{Name: xml.Name{Local: "tv"}}); err != nil {
		return errors.Wrap(err, "failed to write end element")
	}

	return g.encoder.Flush()
}

// writeFile writes the XML content to disk
func (g *XMLTVGenerator) writeFile() error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(Config.Files.XMLTV)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return errors.Wrap(err, "failed to create directory")
	}

	// Write to temporary file first
	tmpFile := Config.Files.XMLTV + ".tmp"
	if err := os.WriteFile(tmpFile, g.buffer.Bytes(), 0644); err != nil {
		return errors.Wrap(err, "failed to write temporary file")
	}

	// Rename temporary file to actual file
	if err := os.Rename(tmpFile, Config.Files.XMLTV); err != nil {
		os.Remove(tmpFile) // Clean up temp file
		return errors.Wrap(err, "failed to rename temporary file")
	}

	return nil
}

// getPrograms gets all programs for a channel
func (g *XMLTVGenerator) getPrograms(channel G2GCache) ([]Programme, error) {
	schedule, ok := Cache.Schedule[channel.StationID]
	if !ok {
		return nil, nil
	}

	var programs []Programme
	countryCode := Config.GetLineupCountry(channel.StationID)
	lang := "en"
	if len(channel.BroadcastLanguage) > 0 {
		lang = channel.BroadcastLanguage[0]
	}

	for _, s := range schedule {
		program, err := g.createProgram(channel, s, countryCode, lang)
		if err != nil {
			g.logger.WithError(err).WithFields(logrus.Fields{
				"channel":   channel.Callsign,
				"programID": s.ProgramID,
			}).Error("Failed to create program")
			continue
		}

		programs = append(programs, program)
	}

	return programs, nil
}

// createProgram creates a program from schedule data
func (g *XMLTVGenerator) createProgram(channel G2GCache, schedule G2GCache, countryCode, lang string) (Programme, error) {
	program := Programme{
		Channel: SanitizeID(channel.Callsign),
	}

	// Set start and stop times
	timeLayout := "2006-01-02 15:04:05 +0000 UTC"
	t, err := time.Parse(timeLayout, schedule.AirDateTime.Format(timeLayout))
	if err != nil {
		return program, errors.Wrap(err, "failed to parse air time")
	}

	dateArray := strings.Fields(t.String())
	offset := " " + dateArray[2]
	program.Start = t.Format("20060102150405") + offset
	program.Stop = t.Add(time.Second*time.Duration(schedule.Duration)).Format("20060102150405") + offset

	// Set title with live/new indicators
	program.Title = Cache.GetTitle(schedule.ProgramID, lang)
	if len(program.Title) > 0 {
		if schedule.LiveTapeDelay == "Live" {
			program.Title[0].Value += " ᴸᶦᵛᵉ"
		} else if schedule.New {
			program.Title[0].Value += " ᴺᵉʷ"
		}
	}

	// Set other fields
	program.SubTitle = Cache.GetSubTitle(schedule.ProgramID, lang)
	program.Desc = Cache.GetDescs(schedule.ProgramID, program.SubTitle.Value)
	program.Credits = Cache.GetCredits(schedule.ProgramID)
	program.Categorys = Cache.GetCategory(schedule.ProgramID)
	program.Language = lang
	program.EpisodeNums = Cache.GetEpisodeNum(schedule.ProgramID)
	program.Icon = Cache.GetIcon(schedule.ProgramID[0:10])
	program.Rating = Cache.GetRating(schedule.ProgramID, countryCode)

	// Set video properties
	for _, v := range schedule.VideoProperties {
		switch strings.ToLower(v) {
		case "hdtv", "sdtv", "uhdtv", "3d":
			program.Video.Quality = strings.ToUpper(v)
		}
	}

	// Set audio properties
	for _, a := range schedule.AudioProperties {
		switch a {
		case "stereo", "dvs":
			program.Audio.Stereo = "stereo"
		case "DD 5.1", "Atmos":
			program.Audio.Stereo = "dolby digital"
		case "Dolby":
			program.Audio.Stereo = "dolby"
		case "dubbed", "mono":
			program.Audio.Stereo = "mono"
		default:
			program.Audio.Stereo = "mono"
		}
	}

	// Set new/previously shown status
	if schedule.New {
		program.New = &New{Value: ""}
	} else {
		program.PreviouslyShown = Cache.GetPreviouslyShown(schedule.ProgramID)
	}

	// Set live status
	if schedule.LiveTapeDelay == "Live" {
		program.Live = &Live{Value: ""}
	}

	return program, nil
}

// SanitizeID replaces forbidden characters with underscores for Plex compatibility
func SanitizeID(id string) string {
	return sanitizeIDRegexp.ReplaceAllString(id, "_")
}
