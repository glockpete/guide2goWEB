// Package main provides Guide2Go, a tool to generate XMLTV files from Schedules Direct JSON API.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

// Configure handles the configuration process for the application
func (app *App) Configure(filename string) error {
	app.Logger.WithField("filename", filename).Info("Starting configuration process")
	ctx := context.Background()
	var menu Menu
	var entry Entry
	var sd SD

	app.Config.File = strings.TrimSuffix(filename, filepath.Ext(filename))

	if err := app.Config.Open(ctx); err != nil {
		app.Logger.WithError(err).Error("Failed to open configuration")
		return errors.Wrap(err, "failed to open configuration")
	}

	sd.Init(app)

	if len(app.Config.Account.Username) != 0 || len(app.Config.Account.Password) != 0 {
		if err := sd.Login(); err != nil {
			app.Logger.WithError(err).Error("Failed to login to Schedules Direct")
			return errors.Wrap(err, "failed to login to Schedules Direct")
		}
		if err := sd.Status(); err != nil {
			app.Logger.WithError(err).Error("Failed to get Schedules Direct status")
			return errors.Wrap(err, "failed to get Schedules Direct status")
		}
	}

	for {
		menu := app.setupMenu()
		selection, entry := app.getMenuSelection(&menu, app)
		app.Logger.WithFields(map[string]interface{}{"selection": selection, "entry": entry.Value}).Info("Menu selection")
		switch selection {
		case 0:
			app.Logger.Info("Saving configuration and exiting")
			return app.saveConfig()
		case 1:
			if err := app.handleAccount(&entry, &sd); err != nil {
				app.Logger.WithError(err).Error("Account handling failed")
				return err
			}
		case 2:
			if err := app.handleAddLineup(&entry, &sd); err != nil {
				app.Logger.WithError(err).Error("Add lineup failed")
				return err
			}
		case 3:
			if err := app.handleRemoveLineup(&entry, &sd); err != nil {
				app.Logger.WithError(err).Error("Remove lineup failed")
				return err
			}
		case 4:
			if err := app.handleManageChannels(&entry, &sd); err != nil {
				app.Logger.WithError(err).Error("Manage channels failed")
				return err
			}
		case 5:
			if err := app.handleCreateXMLTV(&sd, filename); err != nil {
				app.Logger.WithError(err).Error("Create XMLTV failed")
				return err
			}
		}
	}
}

func (app *App) setupMenu() Menu {
	var menu Menu
	var entry Entry
	menu.Entry = make(map[int]Entry)
	menu.Headline = fmt.Sprintf("%s [%s.yaml]", getMsg(0000), app.Config.File)
	menu.Select = getMsg(0001)
	// Exit
	entry.Key = 0
	entry.Value = getMsg(0010)
	menu.Entry[0] = entry
	// Account
	entry.Key = 1
	entry.Value = getMsg(0011)
	menu.Entry[1] = entry
	// Add Lineup
	entry.Key = 2
	entry.Value = getMsg(0012)
	menu.Entry[2] = entry
	// Remove Lineup
	entry.Key = 3
	entry.Value = getMsg(0013)
	menu.Entry[3] = entry
	// Manage Channels
	entry.Key = 4
	entry.Value = getMsg(0014)
	menu.Entry[4] = entry
	// Create XMLTV file
	entry.Key = 5
	entry.Value = fmt.Sprintf("%s [%s]", getMsg(0016), app.Config.Files.XMLTV)
	menu.Entry[5] = entry
	return menu
}

func (app *App) getMenuSelection(menu *Menu, appParam *App) (int, Entry) {
	selection := menu.Show(appParam)
	return selection, menu.Entry[selection]
}

func (app *App) saveConfig() error {
	app.Logger.Info("Saving configuration to file")
	if err := app.Config.Save(); err != nil {
		app.Logger.WithError(err).Error("Failed to save configuration")
		return errors.Wrap(err, "failed to save configuration")
	}
	return nil
}

func (app *App) handleAccount(entry *Entry, sd *SD) error {
	app.Logger.Info("Handling account configuration")
	if len(app.Config.Account.Username) == 0 || len(app.Config.Account.Password) == 0 {
		if err := entry.account(app); err != nil {
			app.Logger.WithError(err).Error("Failed to configure account")
			return errors.Wrap(err, "failed to configure account")
		}
		if err := sd.Login(); err != nil {
			os.RemoveAll(app.Config.File + ".yaml")
			app.Logger.WithError(err).Error("Failed to login with new credentials")
			return errors.Wrap(err, "failed to login with new credentials")
		}
		if err := sd.Status(); err != nil {
			app.Logger.WithError(err).Error("Failed to get status after login")
			return errors.Wrap(err, "failed to get status after login")
		}
	} else {
		if err := entry.account(app); err != nil {
			app.Logger.WithError(err).Error("Failed to configure account")
			return errors.Wrap(err, "failed to configure account")
		}
		if err := sd.Login(); err != nil {
			app.Logger.WithError(err).Error("Failed to login with new credentials")
			return errors.Wrap(err, "failed to login with new credentials")
		}
		if err := sd.Status(); err != nil {
			app.Logger.WithError(err).Error("Failed to get status after login")
			return errors.Wrap(err, "failed to get status after login")
		}
	}
	return nil
}

func (app *App) handleAddLineup(entry *Entry, sd *SD) error {
	app.Logger.Info("Handling add lineup")
	if err := entry.addLineup(sd); err != nil {
		app.Logger.WithError(err).Error("Failed to add lineup")
		return errors.Wrap(err, "failed to add lineup")
	}
	if err := sd.Status(); err != nil {
		app.Logger.WithError(err).Error("Failed to get status after adding lineup")
		return errors.Wrap(err, "failed to get status after adding lineup")
	}
	return nil
}

func (app *App) handleRemoveLineup(entry *Entry, sd *SD) error {
	app.Logger.Info("Handling remove lineup")
	if err := entry.removeLineup(sd); err != nil {
		app.Logger.WithError(err).Error("Failed to remove lineup")
		return errors.Wrap(err, "failed to remove lineup")
	}
	if err := sd.Status(); err != nil {
		app.Logger.WithError(err).Error("Failed to get status after removing lineup")
		return errors.Wrap(err, "failed to get status after removing lineup")
	}
	return nil
}

func (app *App) handleManageChannels(entry *Entry, sd *SD) error {
	app.Logger.Info("Handling manage channels")
	if err := entry.manageChannels(sd); err != nil {
		app.Logger.WithError(err).Error("Failed to manage channels")
		return errors.Wrap(err, "failed to manage channels")
	}
	if err := sd.Status(); err != nil {
		app.Logger.WithError(err).Error("Failed to get status after managing channels")
		return errors.Wrap(err, "failed to get status after managing channels")
	}
	return nil
}

func (app *App) handleCreateXMLTV(sd *SD, filename string) error {
	app.Logger.WithField("filename", filename).Info("Handling create XMLTV")
	if err := sd.Update(filename); err != nil {
		app.Logger.WithError(err).Error("Failed to update EPG data")
		return errors.Wrap(err, "failed to update EPG data")
	}
	return nil
}

// Open opens and validates the configuration file
func (c *config) Open(ctx context.Context) error {
	data, err := os.ReadFile(fmt.Sprintf("%s.yaml", c.File))
	if err != nil {
		// File is missing, create new config file
		c.InitConfig()
		return c.Save()
	}

	// Open config file and convert YAML to struct
	if err := yaml.Unmarshal(data, &c); err != nil {
		return errors.Wrap(err, "failed to parse configuration file")
	}

	// Validate configuration
	if err := c.validate(); err != nil {
		return errors.Wrap(err, "invalid configuration")
	}

	// Update configuration with new options if needed
	if err := c.updateNewOptions(data); err != nil {
		return errors.Wrap(err, "failed to update configuration with new options")
	}

	return nil
}

// Save saves the configuration to file with proper permissions
func (c *config) Save() error {
	data, err := yaml.Marshal(&c)
	if err != nil {
		return errors.Wrap(err, "failed to marshal configuration")
	}

	// Create a temporary file
	tmpFile := fmt.Sprintf("%s.yaml.tmp", c.File)
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return errors.Wrap(err, "failed to write temporary configuration file")
	}

	// Rename temporary file to actual file
	if err := os.Rename(tmpFile, fmt.Sprintf("%s.yaml", c.File)); err != nil {
		os.Remove(tmpFile) // Clean up temp file
		return errors.Wrap(err, "failed to rename temporary configuration file")
	}

	return nil
}

// InitConfig initializes a new configuration with default values
func (c *config) InitConfig() {
	// Generate a secure random token for API authentication
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		logger.WithError(err).Warn("Failed to generate secure token, using fallback")
		token = []byte(time.Now().String())
	}

	// Files
	c.Files.Cache = fmt.Sprintf("%s_cache.json", c.File)
	c.Files.XMLTV = fmt.Sprintf("%s.xml", c.File)

	// Options
	c.Options.PosterAspect = "landscape"
	c.Options.Schedule = 7
	c.Options.SubtitleIntoDescription = true
	c.Options.Credits = true
	c.Options.TVShowImages = false
	c.Options.ImagesPath = "${images_path}"
	c.Options.ProxyImages = false
	c.Options.Hostname = "localhost:8080"
	c.Options.CacheExpiration = 24 * time.Hour
	c.Options.SDDownloadErrors = false

	// Rating
	c.Options.Rating.Guidelines = true
	c.Options.Rating.MaxEntries = 1
	c.Options.Rating.Countries = []string{}
	c.Options.Rating.CountryCodeAsSystem = false
}

// validate performs validation on the configuration
func (c *config) validate() error {
	// Validate required fields
	if c.Files.Cache == "" {
		return errors.New("cache file path is required")
	}
	if c.Files.XMLTV == "" {
		return errors.New("XMLTV file path is required")
	}
	if c.Options.ImagesPath == "" {
		return errors.New("images path is required")
	}
	if c.Options.Hostname == "" {
		return errors.New("hostname is required")
	}

	// Validate schedule days
	if c.Options.Schedule < 1 || c.Options.Schedule > 14 {
		return errors.New("schedule days must be between 1 and 14")
	}

	// Validate poster aspect
	switch c.Options.PosterAspect {
	case "portrait", "landscape", "square":
		// Valid values
	default:
		return errors.New("invalid poster aspect")
	}

	// Validate rating entries
	if c.Options.Rating.MaxEntries < 0 || c.Options.Rating.MaxEntries > 10 {
		return errors.New("rating max entries must be between 0 and 10")
	}

	return nil
}

// updateNewOptions updates the configuration with new options if needed
func (c *config) updateNewOptions(data []byte) error {
	var updated bool

	// Check and update new options
	if !bytes.Contains(data, []byte("credits tag")) {
		updated = true
		c.Options.Credits = true
		logger.Info("Added credits tag option")
	}

	if !bytes.Contains(data, []byte("Rating:")) {
		updated = true
		c.Options.Rating.Guidelines = true
		c.Options.Rating.Countries = []string{}
		c.Options.Rating.CountryCodeAsSystem = false
		c.Options.Rating.MaxEntries = 1
		logger.Info("Added rating options")
	}

	if !bytes.Contains(data, []byte("Local Images Cache:")) {
		updated = true
		c.Options.TVShowImages = false
		logger.Info("Added local images cache option")
	}

	if !bytes.Contains(data, []byte("Images Path:")) {
		updated = true
		c.Options.ImagesPath = "${images_path}"
		logger.Info("Added images path option")
	}

	if !bytes.Contains(data, []byte("Proxy Images")) {
		updated = true
		c.Options.ProxyImages = false
		logger.Info("Added proxy images option")
	}

	if !bytes.Contains(data, []byte("Hostname")) {
		updated = true
		c.Options.Hostname = "localhost:8080"
		logger.Info("Added hostname option")
	}

	if !bytes.Contains(data, []byte("download errors")) {
		updated = true
		c.Options.SDDownloadErrors = false
		logger.Info("Added SD download errors option")
	}

	if !bytes.Contains(data, []byte("Cache Expiration")) {
		updated = true
		c.Options.CacheExpiration = 24 * time.Hour
		logger.Info("Added cache expiration option")
	}

	if updated {
		return c.Save()
	}

	return nil
}

func (c *config) GetChannelList(lineup string) (list []string) {

	for _, channel := range c.Station {

		switch len(lineup) {

		case 0:
			list = append(list, channel.ID)

		default:
			if lineup == channel.Lineup {
				list = append(list, channel.ID)
			}

		}

	}

	return
}

func (c *config) GetLineupCountry(id string) (countryCode string) {

	for _, channel := range c.Station {

		if id == channel.ID {
			countryCode = strings.Split(channel.Lineup, "-")[0]
			return
		}

	}

	return
}
