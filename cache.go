// Package main provides Guide2Go, a tool to generate XMLTV files from Schedules Direct JSON API.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	defaultCacheExpiration = 24 * time.Hour
	maxCacheSize           = 100 * 1024 * 1024 // 100MB
)

// Cache represents the global cache instance
var ImageError bool = false

var bufferPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, 32*1024) // 32KB buffer
	},
}

var httpClient = &http.Client{}

// Data struct for metadata (restored from struct_sd.go)
type Data struct {
	Aspect   string `json:"aspect"`
	Height   string `json:"height"`
	Size     string `json:"size"`
	URI      string `json:"uri"`
	Width    string `json:"width"`
	Category string `json:"category"`
	Tier     string `json:"tier"`
}

// SDProgram struct for program data (restored from struct_sd.go)
type SDProgram struct {
	Cast []struct {
		BillingOrder  string `json:"billingOrder"`
		CharacterName string `json:"characterName"`
		Name          string `json:"name"`
		NameID        string `json:"nameId"`
		PersonID      string `json:"personId"`
		Role          string `json:"role"`
	} `json:"cast"`
	ContentAdvisory []string `json:"contentAdvisory"`
	ContentRating   []struct {
		Body    string `json:"body"`
		Code    string `json:"code"`
		Country string `json:"country"`
	} `json:"contentRating"`
	Crew []struct {
		BillingOrder string `json:"billingOrder"`
		Name         string `json:"name"`
		NameID       string `json:"nameId"`
		PersonID     string `json:"personId"`
		Role         string `json:"role"`
	} `json:"crew"`
	Descriptions struct {
		Description1000 []struct {
			Description         string `json:"description"`
			DescriptionLanguage string `json:"descriptionLanguage"`
		} `json:"description1000"`
		Description100 []struct {
			DescriptionLanguage string `json:"descriptionLanguage"`
			Description         string `json:"description"`
		} `json:"description100"`
	} `json:"descriptions"`
	EntityType        string   `json:"entityType"`
	EpisodeTitle150   string   `json:"episodeTitle150"`
	Genres            []string `json:"genres"`
	HasEpisodeArtwork bool     `json:"hasEpisodeArtwork"`
	HasImageArtwork   bool     `json:"hasImageArtwork"`
	HasSeriesArtwork  bool     `json:"hasSeriesArtwork"`
	Md5               string   `json:"md5"`
	Metadata          []struct {
		Gracenote struct {
			Episode int `json:"episode"`
			Season  int `json:"season"`
		} `json:"Gracenote"`
	} `json:"metadata"`
	OriginalAirDate string `json:"originalAirDate"`
	ProgramID       string `json:"programID"`
	ResourceID      string `json:"resourceID"`
	ShowType        string `json:"showType"`
	Titles          []struct {
		Title120 string `json:"title120"`
	} `json:"titles"`
}

// SDMetadata struct for metadata (restored from struct_sd.go)
type SDMetadata struct {
	Data      []Data `json:"data"`
	ProgramID string `json:"programID"`
}

// SDError struct for error responses from SD (restored from struct_sd.go)
type SDError struct {
	Data struct {
		Code     int64  `json:"code"`
		Datetime string `json:"datetime"`
		Message  string `json:"message"`
		Response string `json:"response"`
		ServerID string `json:"serverID"`
	} `json:"data"`
	ProgramID string `json:"programID"`
}

// G2GCache : Cache data
// Restored from struct_cache.go
// This struct is used for caching channel, program, metadata, and schedule data.
type G2GCache struct {
	// Global
	Md5       string `json:"md5,omitempty"`
	ProgramID string `json:"programID,omitempty"`

	// Channel
	StationID         string   `json:"stationID,omitempty"`
	Name              string   `json:"name,omitempty"`
	Callsign          string   `json:"callsign,omitempty"`
	Affiliate         string   `json:"affiliate,omitempty"`
	BroadcastLanguage []string `json:"broadcastLanguage"`
	StationLogo       []struct {
		URL    string `json:"URL"`
		Height int    `json:"height"`
		Width  int    `json:"width"`
		Md5    string `json:"md5"`
		Source string `json:"source"`
	} `json:"stationLogo,omitempty"`
	Logo struct {
		URL    string `json:"URL"`
		Height int    `json:"height"`
		Width  int    `json:"width"`
		Md5    string `json:"md5"`
	} `json:"logo,omitempty"`

	// Schedule
	AirDateTime     time.Time `json:"airDateTime,omitempty"`
	AudioProperties []string  `json:"audioProperties,omitempty"`
	Duration        int       `json:"duration,omitempty"`
	LiveTapeDelay   string    `json:"liveTapeDelay,omitempty"`
	New             bool      `json:"new,omitempty"`
	Ratings         []struct {
		Body string `json:"body"`
		Code string `json:"code"`
	} `json:"ratings,omitempty"`
	VideoProperties []string `json:"videoProperties,omitempty"`

	// Program
	Cast []struct {
		BillingOrder  string `json:"billingOrder"`
		CharacterName string `json:"characterName"`
		Name          string `json:"name"`
		NameID        string `json:"nameId"`
		PersonID      string `json:"personId"`
		Role          string `json:"role"`
	} `json:"cast"`
	Crew []struct {
		BillingOrder string `json:"billingOrder"`
		Name         string `json:"name"`
		NameID       string `json:"nameId"`
		PersonID     string `json:"personId"`
		Role         string `json:"role"`
	} `json:"crew"`
	ContentRating []struct {
		Body    string `json:"body"`
		Code    string `json:"code"`
		Country string `json:"country"`
	} `json:"contentRating"`
	Descriptions struct {
		Description1000 []struct {
			Description         string `json:"description"`
			DescriptionLanguage string `json:"descriptionLanguage"`
		} `json:"description1000"`
		Description100 []struct {
			DescriptionLanguage string `json:"descriptionLanguage"`
			Description         string `json:"description"`
		} `json:"description100"`
	} `json:"descriptions"`

	EpisodeTitle150   string   `json:"episodeTitle150,omitempty"`
	Genres            []string `json:"genres,omitempty"`
	HasEpisodeArtwork bool     `json:"hasEpisodeArtwork,omitempty"`
	HasImageArtwork   bool     `json:"hasImageArtwork,omitempty"`
	HasSeriesArtwork  bool     `json:"hasSeriesArtwork,omitempty"`

	Metadata []struct {
		Gracenote struct {
			Episode int `json:"episode"`
			Season  int `json:"season"`
		} `json:"Gracenote"`
	} `json:"metadata,omitempty"`

	OriginalAirDate string `json:"originalAirDate,omitempty"`
	ResourceID      string `json:"resourceID,omitempty"`
	ShowType        string `json:"showType,omitempty"`
	Titles          []struct {
		Title120 string `json:"title120"`
	} `json:"titles"`

	// Metadata
	Data []Data `json:"data,omitempty"`
}

// cache represents the application's cache system
type cache struct {
	Channel  map[string]G2GCache   `json:"Channel"`
	Program  map[string]G2GCache   `json:"Program"`
	Metadata map[string]G2GCache   `json:"Metadata"`
	Schedule map[string][]G2GCache `json:"Schedule"`

	stats struct {
		Hits   int64 `json:"hits"`
		Misses int64 `json:"misses"`
		Size   int64 `json:"size"`
	}

	expiration time.Time `json:"expiration"`
	sync.RWMutex
}

// CacheStore defines the interface for cache operations
// This allows for easier testing and mocking.
type CacheStore interface {
	Open(app *App) error
	Save(app *App) error
	Init()
	CleanUp(app *App)
	GetTitle(id, lang string, app *App) []Title
	GetSubTitle(id, lang string, app *App) SubTitle
	GetDescs(id, subTitle string, app *App) []Desc
	GetCredits(id string, app *App) Credits
	GetCategory(id string, app *App) []Category
	GetEpisodeNum(id string, app *App) []EpisodeNum
	GetIcon(id string, app *App) []Icon
	GetRating(id, countryCode string, app *App) []Rating
	GetPreviouslyShown(id string, app *App) *PreviouslyShown
	AddStations(ctx context.Context, data *[]byte, lineup string, app *App) error
	AddSchedule(ctx context.Context, data *[]byte, app *App) error
	AddProgram(ctx context.Context, gzip *[]byte, wg *sync.WaitGroup, app *App) error
	AddMetadata(ctx context.Context, gzip *[]byte, wg *sync.WaitGroup, app *App) error
}

// Init initializes the cache with default values
func (c *cache) Init() {
	c.Lock()
	defer c.Unlock()

	if c.Schedule == nil {
		c.Schedule = make(map[string][]G2GCache)
	}
	if c.Channel == nil {
		c.Channel = make(map[string]G2GCache)
	}
	if c.Program == nil {
		c.Program = make(map[string]G2GCache)
	}
	if c.Metadata == nil {
		c.Metadata = make(map[string]G2GCache)
	}

	c.expiration = time.Now().Add(defaultCacheExpiration)
}

// Remove removes the cache file and reinitializes the cache
func (c *cache) Remove(app *App) error {
	c.Lock()
	defer c.Unlock()

	if len(app.Config.Files.Cache) == 0 {
		return errors.New("cache file path not configured")
	}

	app.Logger.WithField("path", app.Config.Files.Cache).Info("Removing cache file")
	if err := os.RemoveAll(app.Config.Files.Cache); err != nil {
		return errors.Wrap(err, "failed to remove cache file")
	}

	c.Init()
	return nil
}

// AddStations adds station data to the cache
func (c *cache) AddStations(ctx context.Context, data *[]byte, lineup string, app *App) error {
	c.Lock()
	defer c.Unlock()

	var g2gCache G2GCache
	var sdData SDStation

	if err := json.Unmarshal(*data, &sdData); err != nil {
		return errors.Wrap(err, "failed to unmarshal station data")
	}

	channelIDs := app.Config.GetChannelList(lineup)
	added := 0

	for _, sd := range sdData.Stations {
		if ContainsString(channelIDs, sd.StationID) != -1 {
			g2gCache = G2GCache{
				StationID:         sd.StationID,
				Name:              sd.Name,
				Callsign:          sd.Callsign,
				Affiliate:         sd.Affiliate,
				BroadcastLanguage: sd.BroadcastLanguage,
				Logo:              sd.Logo,
			}

			c.Channel[sd.StationID] = g2gCache
			added++
		}
	}

	app.Logger.WithFields(logrus.Fields{
		"lineup": lineup,
		"added":  added,
	}).Debug("Added stations to cache")

	return nil
}

// AddSchedule adds schedule data to the cache
func (c *cache) AddSchedule(ctx context.Context, data *[]byte, app *App) error {
	c.Lock()
	defer c.Unlock()

	var g2gCache G2GCache
	var sdData []SDSchedule

	if err := json.Unmarshal(*data, &sdData); err != nil {
		return errors.Wrap(err, "failed to unmarshal schedule data")
	}

	added := 0
	for _, sd := range sdData {
		if _, ok := c.Schedule[sd.StationID]; !ok {
			c.Schedule[sd.StationID] = []G2GCache{}
		}

		for _, p := range sd.Programs {
			g2gCache = G2GCache{
				AirDateTime:     p.AirDateTime,
				AudioProperties: p.AudioProperties,
				Duration:        p.Duration,
				LiveTapeDelay:   p.LiveTapeDelay,
				New:             p.New,
				Md5:             p.Md5,
				ProgramID:       p.ProgramID,
				Ratings:         p.Ratings,
				VideoProperties: p.VideoProperties,
			}

			c.Schedule[sd.StationID] = append(c.Schedule[sd.StationID], g2gCache)
			added++
		}
	}

	app.Logger.WithField("added", added).Debug("Added schedule data to cache")
	return nil
}

// AddProgram adds program data to the cache
func (c *cache) AddProgram(ctx context.Context, gzip *[]byte, wg *sync.WaitGroup, app *App) error {
	defer wg.Done()

	c.Lock()
	defer c.Unlock()

	b, err := gUnzip(*gzip)
	if err != nil {
		return errors.Wrap(err, "failed to decompress program data")
	}

	var g2gCache G2GCache
	var sdData []SDProgram

	if err := json.Unmarshal(b, &sdData); err != nil {
		return errors.Wrap(err, "failed to unmarshal program data")
	}

	added := 0
	for _, sd := range sdData {
		g2gCache = G2GCache{
			Descriptions:      sd.Descriptions,
			EpisodeTitle150:   sd.EpisodeTitle150,
			Genres:            sd.Genres,
			HasEpisodeArtwork: sd.HasEpisodeArtwork,
			HasImageArtwork:   sd.HasImageArtwork,
			HasSeriesArtwork:  sd.HasSeriesArtwork,
			Metadata:          sd.Metadata,
			OriginalAirDate:   sd.OriginalAirDate,
			ResourceID:        sd.ResourceID,
			ShowType:          sd.ShowType,
			Titles:            sd.Titles,
			ContentRating:     sd.ContentRating,
			Cast:              sd.Cast,
			Crew:              sd.Crew,
		}

		c.Program[sd.ProgramID] = g2gCache
		added++
	}

	app.Logger.WithField("added", added).Debug("Added program data to cache")
	return nil
}

// AddMetadata adds metadata to the cache
func (c *cache) AddMetadata(ctx context.Context, gzip *[]byte, wg *sync.WaitGroup, app *App) error {
	defer wg.Done()

	c.Lock()
	defer c.Unlock()

	b, err := gUnzip(*gzip)
	if err != nil {
		return errors.Wrap(err, "failed to decompress metadata")
	}

	var tmp = make([]interface{}, 0)
	if err := json.Unmarshal(b, &tmp); err != nil {
		return errors.Wrap(err, "failed to unmarshal metadata")
	}

	added := 0
	for _, t := range tmp {
		var sdData SDMetadata
		jsonByte, _ := json.Marshal(t)

		if err := json.Unmarshal(jsonByte, &sdData); err != nil {
			var sdError SDError
			if err := json.Unmarshal(jsonByte, &sdError); err == nil && app.Config.Options.SDDownloadErrors {
				app.Logger.WithFields(logrus.Fields{
					"code":      sdError.Data.Code,
					"message":   sdError.Data.Message,
					"programID": sdError.ProgramID,
				}).Error("SD API error")
			}
			continue
		}

		c.Metadata[sdData.ProgramID] = G2GCache{Data: sdData.Data}
		added++
	}

	app.Logger.WithField("added", added).Debug("Added metadata to cache")
	return nil
}

// Open loads the cache from disk
func (c *cache) Open(app *App) error {
	c.Lock()
	defer c.Unlock()

	if len(app.Config.Files.Cache) == 0 {
		return errors.New("cache file path not configured")
	}

	data, err := os.ReadFile(app.Config.Files.Cache)
	if err != nil {
		if os.IsNotExist(err) {
			c.Init()
			return nil
		}
		return errors.Wrap(err, "failed to read cache file")
	}

	if err := json.Unmarshal(data, c); err != nil {
		return errors.Wrap(err, "failed to unmarshal cache data")
	}

	// Check cache expiration
	if time.Now().After(c.expiration) {
		app.Logger.Info("Cache expired, reinitializing")
		c.Init()
		return nil
	}

	return nil
}

// Save persists the cache to disk
func (c *cache) Save(app *App) error {
	c.Lock()
	defer c.Unlock()

	if len(app.Config.Files.Cache) == 0 {
		return errors.New("cache file path not configured")
	}

	// Create cache directory if it doesn't exist
	dir := filepath.Dir(app.Config.Files.Cache)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return errors.Wrap(err, "failed to create cache directory")
	}

	// Marshal cache data
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal cache data")
	}

	// Write to temporary file first
	tmpFile := app.Config.Files.Cache + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return errors.Wrap(err, "failed to write temporary cache file")
	}

	// Rename temporary file to actual file
	if err := os.Rename(tmpFile, app.Config.Files.Cache); err != nil {
		os.Remove(tmpFile) // Clean up temp file
		return errors.Wrap(err, "failed to rename temporary cache file")
	}

	return nil
}

// CleanUp removes expired entries from the cache
func (c *cache) CleanUp(app *App) {
	c.Lock()
	defer c.Unlock()

	now := time.Now()
	expired := 0

	// Clean up schedules
	for stationID, schedules := range c.Schedule {
		var validSchedules []G2GCache
		for _, schedule := range schedules {
			if schedule.AirDateTime.After(now) {
				validSchedules = append(validSchedules, schedule)
			} else {
				expired++
			}
		}
		if len(validSchedules) == 0 {
			delete(c.Schedule, stationID)
		} else {
			c.Schedule[stationID] = validSchedules
		}
	}

	// Clean up programs
	for programID, program := range c.Program {
		if program.OriginalAirDate != "" {
			airDate, err := time.Parse("2006-01-02", program.OriginalAirDate)
			if err == nil && airDate.Before(now.AddDate(0, -1, 0)) {
				delete(c.Program, programID)
				expired++
			}
		}
	}

	app.Logger.WithField("expired", expired).Info("Cleaned up cache")
}

// GetStats returns cache statistics
func (c *cache) GetStats() map[string]interface{} {
	c.RLock()
	defer c.RUnlock()

	return map[string]interface{}{
		"hits":     c.stats.Hits,
		"misses":   c.stats.Misses,
		"size":     c.stats.Size,
		"channels": len(c.Channel),
		"programs": len(c.Program),
		"metadata": len(c.Metadata),
		"schedule": len(c.Schedule),
		"expires":  c.expiration,
	}
}

// Get data from cache
func (c *cache) GetTitle(id, lang string, app *App) (t []Title) {

	if p, ok := c.Program[id]; ok {

		var title Title

		for _, s := range p.Titles {
			title.Value = s.Title120
			title.Lang = lang
			t = append(t, title)
		}

	}

	if len(t) == 0 {
		var title Title
		title.Value = "No EPG Info"
		title.Lang = "en"
		t = append(t, title)
	}

	return
}

func (c *cache) GetSubTitle(id, lang string, app *App) (s SubTitle) {

	if p, ok := c.Program[id]; ok {

		if len(p.EpisodeTitle150) != 0 {

			s.Value = p.EpisodeTitle150
			s.Lang = lang

		} else {

			for _, d := range p.Descriptions.Description100 {

				s.Value = d.Description
				s.Lang = d.DescriptionLanguage

			}

		}

	}

	return
}

func (c *cache) GetDescs(id, subTitle string, app *App) (de []Desc) {

	if p, ok := c.Program[id]; ok {

		d := p.Descriptions

		var desc Desc

		for _, tmp := range d.Description1000 {

			switch app.Config.Options.SubtitleIntoDescription {

			case true:
				if len(subTitle) != 0 {
					desc.Value = fmt.Sprintf("[%s]\n%s", subTitle, tmp.Description)
					break
				}

				fallthrough
			case false:
				desc.Value = tmp.Description

			}

			desc.Lang = tmp.DescriptionLanguage

			de = append(de, desc)
		}

	}

	return
}

func (c *cache) GetCredits(id string, app *App) (cr Credits) {

	if app.Config.Options.Credits {

		if p, ok := c.Program[id]; ok {

			// Crew
			for _, crew := range p.Crew {

				switch crew.Role {

				case "Director":
					cr.Director = append(cr.Director, Director{Value: crew.Name})

				case "Producer":
					cr.Producer = append(cr.Producer, Producer{Value: crew.Name})

				case "Presenter":
					cr.Presenter = append(cr.Presenter, Presenter{Value: crew.Name})

				case "Writer":
					cr.Writer = append(cr.Writer, Writer{Value: crew.Name})

				}

			}

			// Cast
			for _, cast := range p.Cast {

				switch cast.Role {

				case "Actor":
					cr.Actor = append(cr.Actor, Actor{Value: cast.Name, Role: cast.CharacterName})

				}

			}

		}

	}

	return
}

func (c *cache) GetCategory(id string, app *App) (ca []Category) {

	if p, ok := c.Program[id]; ok {

		for _, g := range p.Genres {

			var category Category
			category.Value = g
			category.Lang = "en"

			ca = append(ca, category)

		}

	}

	return
}

func (c *cache) GetEpisodeNum(id string, app *App) (ep []EpisodeNum) {

	var seaseon, episode int

	if p, ok := c.Program[id]; ok {

		for _, m := range p.Metadata {

			seaseon = m.Gracenote.Season
			episode = m.Gracenote.Episode

			var episodeNum EpisodeNum

			if seaseon != 0 && episode != 0 {

				episodeNum.Value = fmt.Sprintf("%d.%d.", seaseon-1, episode-1)
				episodeNum.System = "xmltv_ns"

				ep = append(ep, episodeNum)
			}

		}

		if seaseon != 0 && episode != 0 {

			var episodeNum EpisodeNum
			episodeNum.Value = fmt.Sprintf("S%d E%d", seaseon, episode)
			episodeNum.System = "onscreen"
			ep = append(ep, episodeNum)

		}

		if len(ep) == 0 {

			var episodeNum EpisodeNum

			switch id[0:2] {

			case "EP":
				episodeNum.Value = id[0:10] + "." + id[10:]

			case "SH", "MV":
				episodeNum.Value = id[0:10] + ".0000"

			default:
				episodeNum.Value = id
			}

			episodeNum.System = "dd_progid"

			ep = append(ep, episodeNum)

		}

		if len(p.OriginalAirDate) > 0 {

			var episodeNum EpisodeNum
			episodeNum.Value = p.OriginalAirDate
			episodeNum.System = "original-air-date"
			ep = append(ep, episodeNum)

		}

	}

	return
}

func (c *cache) GetPreviouslyShown(id string, app *App) (prev *PreviouslyShown) {

	prev = &PreviouslyShown{}

	if p, ok := c.Program[id]; ok {
		prev.Start = p.OriginalAirDate
	}

	return
}

// GetImageUrl downloads an image from Schedules Direct and saves it locally.
// It skips download if the image already exists and is valid.
func (app *App) GetImageUrl(urlid string, name string) error {
	url := urlid + "?token=" + app.Token
	filename := app.Config.Options.ImagesPath + name

	a, err := os.Stat(filename)
	if err == nil && a.Size() >= 500 {
		// File exists and is valid
		return nil
	}

	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", filename, err)
	}
	defer file.Close()

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request for %s: %w", url, err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch image from %s: %w", url, err)
	}
	defer resp.Body.Close()

	buf := bufferPool.Get().([]byte)
	defer bufferPool.Put(buf)
	if _, err := io.CopyBuffer(file, resp.Body, buf); err != nil {
		return fmt.Errorf("failed to write image to %s: %w", filename, err)
	}

	info, err := os.Stat(filename)
	if err != nil {
		return fmt.Errorf("failed to stat file %s after download: %w", filename, err)
	}
	if info.Size() < 500 {
		return fmt.Errorf("downloaded image %s is too small (%d bytes)", filename, info.Size())
	}

	return nil
}

func (c *cache) GetIcon(id string, app *App) (i []Icon) {

	var aspects = []string{"2x3", "4x3", "3x4", "16x9"}
	var uri string
	var width, height int
	var err error
	var nameFinal string
	switch app.Config.Options.PosterAspect {

	case "all":
		break

	default:
		aspects = []string{app.Config.Options.PosterAspect}

	}

	if m, ok := c.Metadata[id]; ok {
		var nameTemp string
		for _, aspect := range aspects {
			var maxWidth, maxHeight int
			var finalCategory string = ""
			for _, icon := range m.Data {
				if finalCategory == "" && (icon.Category == "Poster Art" || icon.Category == "Box Art" || icon.Category == "Banner-L1" || icon.Category == "Banner-L2") {
					finalCategory = icon.Category
				} else if finalCategory == "" && icon.Category == "VOD Art" {
					finalCategory = icon.Category
				}
				if icon.Category != finalCategory {
					continue
				}

				if icon.URI[0:7] != "http://" && icon.URI[0:8] != "https://" {
					nameTemp = icon.URI
					icon.URI = fmt.Sprintf("https://json.schedulesdirect.org/20141201/image/%s", icon.URI)
				}

				if icon.Aspect == aspect {

					width, err = strconv.Atoi(icon.Width)
					if err != nil {
						return
					}

					height, err = strconv.Atoi(icon.Height)
					if err != nil {
						return
					}

					if width > maxWidth {
						maxWidth = width
						maxHeight = height
						uri = icon.URI
						nameFinal = nameTemp
					}

				}

			}

			if maxWidth > 0 {
				if app.Config.Options.TVShowImages {
					err := app.GetImageUrl(uri, nameFinal)
					if err != nil {
						app.Logger.WithError(err).WithFields(logrus.Fields{
							"uri":  uri,
							"name": nameFinal,
						}).Error("Failed to download image")
						continue
					}
				}
				path := "http://" + app.Config.Options.Hostname + "/images/" + nameFinal
				i = append(i, Icon{Src: path, Height: maxHeight, Width: maxWidth})
			}

		}

	}

	return
}

func (c *cache) GetRating(id, countryCode string, app *App) (ra []Rating) {

	if !app.Config.Options.Rating.Guidelines {
		return
	}

	var add = func(code, body, country string) {

		switch app.Config.Options.Rating.CountryCodeAsSystem {

		case true:
			ra = append(ra, Rating{Value: code, System: country})

		case false:
			ra = append(ra, Rating{Value: code, System: body})

		}

	}

	/*
	   var prepend = func(code, body, country string) {

	     switch app.Config.Options.Rating.CountryCodeAsSystem {

	     case true:
	       ra = append([]Rating{{Value: code, System: country}}, ra...)

	     case false:
	       ra = append([]Rating{{Value: code, System: body}}, ra...)

	     }

	   }
	*/

	if p, ok := c.Program[id]; ok {

		switch len(app.Config.Options.Rating.Countries) {

		case 0:
			for _, r := range p.ContentRating {

				if len(ra) == app.Config.Options.Rating.MaxEntries && app.Config.Options.Rating.MaxEntries != 0 {
					return
				}

				if countryCode == r.Country {
					add(r.Code, r.Body, r.Country)
				}

			}

			for _, r := range p.ContentRating {

				if len(ra) == app.Config.Options.Rating.MaxEntries && app.Config.Options.Rating.MaxEntries != 0 {
					return
				}

				if countryCode != r.Country {
					add(r.Code, r.Body, r.Country)
				}

			}

		default:
			for _, cCode := range app.Config.Options.Rating.Countries {

				for _, r := range p.ContentRating {

					if len(ra) == app.Config.Options.Rating.MaxEntries && app.Config.Options.Rating.MaxEntries != 0 {
						return
					}

					if cCode == r.Country {

						add(r.Code, r.Body, r.Country)

					}

				}

			}

		}

	}

	return
}

// SDStation struct for station data (restored from struct_sd.go)
type SDStation struct {
	Map []struct {
		Channel   string `json:"channel"`
		StationID string `json:"stationID"`
	} `json:"map"`
	Metadata struct {
		Lineup    string `json:"lineup"`
		Modified  string `json:"modified"`
		Transport string `json:"transport"`
	} `json:"metadata"`
	Stations []struct {
		Affiliate         string   `json:"affiliate"`
		BroadcastLanguage []string `json:"broadcastLanguage"`
		Broadcaster       struct {
			City       string `json:"city"`
			Country    string `json:"country"`
			Postalcode string `json:"postalcode"`
			State      string `json:"state"`
		} `json:"broadcaster"`
		Callsign            string   `json:"callsign"`
		DescriptionLanguage []string `json:"descriptionLanguage"`
		Logo                struct {
			URL    string `json:"URL"`
			Height int    `json:"height"`
			Width  int    `json:"width"`
			Md5    string `json:"md5"`
		} `json:"logo,omitempty"`
		Name        string `json:"name"`
		StationID   string `json:"stationID"`
		StationLogo []struct {
			URL    string `json:"URL"`
			Height int    `json:"height"`
			Width  int    `json:"width"`
			Md5    string `json:"md5"`
		} `json:"stationLogo,omitempty"`
	} `json:"stations"`
}

// Restore SDSchedule struct for schedule data (restored from struct_cache.go)
type SDSchedule struct {
	Programs []struct {
		AirDateTime     time.Time `json:"airDateTime"`
		AudioProperties []string  `json:"audioProperties"`
		Duration        int       `json:"duration"`
		LiveTapeDelay   string    `json:"liveTapeDelay"`
		New             bool      `json:"new"`
		Md5             string    `json:"md5"`
		ProgramID       string    `json:"programID"`
		Ratings         []struct {
			Body string `json:"body"`
			Code string `json:"code"`
		} `json:"ratings"`
		VideoProperties []string `json:"videoProperties"`
	} `json:"programs"`
	StationID string `json:"stationID"`
}
