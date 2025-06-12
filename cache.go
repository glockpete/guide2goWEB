// Package main provides Guide2Go, a tool to generate XMLTV files from Schedules Direct JSON API.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	defaultCacheExpiration = 24 * time.Hour
	maxCacheSize          = 100 * 1024 * 1024 // 100MB
)

// Cache represents the global cache instance
var Cache cache
var ImageError bool = false

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
func (c *cache) Remove() error {
	c.Lock()
	defer c.Unlock()

	if len(Config.Files.Cache) == 0 {
		return errors.New("cache file path not configured")
	}

	logger.WithField("path", Config.Files.Cache).Info("Removing cache file")
	if err := os.RemoveAll(Config.Files.Cache); err != nil {
		return errors.Wrap(err, "failed to remove cache file")
	}

	c.Init()
	return nil
}

// AddStations adds station data to the cache
func (c *cache) AddStations(ctx context.Context, data *[]byte, lineup string) error {
	c.Lock()
	defer c.Unlock()

	var g2gCache G2GCache
	var sdData SDStation

	if err := json.Unmarshal(*data, &sdData); err != nil {
		return errors.Wrap(err, "failed to unmarshal station data")
	}

	channelIDs := Config.GetChannelList(lineup)
	added := 0

	for _, sd := range sdData.Stations {
		if ContainsString(channelIDs, sd.StationID) != -1 {
			g2gCache = G2GCache{
				StationID:         sd.StationID,
				Name:             sd.Name,
				Callsign:         sd.Callsign,
				Affiliate:        sd.Affiliate,
				BroadcastLanguage: sd.BroadcastLanguage,
				Logo:             sd.Logo,
			}

			c.Channel[sd.StationID] = g2gCache
			added++
		}
	}

	logger.WithFields(logrus.Fields{
		"lineup": lineup,
		"added":  added,
	}).Debug("Added stations to cache")

	return nil
}

// AddSchedule adds schedule data to the cache
func (c *cache) AddSchedule(ctx context.Context, data *[]byte) error {
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

	logger.WithField("added", added).Debug("Added schedule data to cache")
	return nil
}

// AddProgram adds program data to the cache
func (c *cache) AddProgram(ctx context.Context, gzip *[]byte, wg *sync.WaitGroup) error {
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

	logger.WithField("added", added).Debug("Added program data to cache")
	return nil
}

// AddMetadata adds metadata to the cache
func (c *cache) AddMetadata(ctx context.Context, gzip *[]byte, wg *sync.WaitGroup) error {
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
			if err := json.Unmarshal(jsonByte, &sdError); err == nil && Config.Options.SDDownloadErrors {
				logger.WithFields(logrus.Fields{
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

	logger.WithField("added", added).Debug("Added metadata to cache")
	return nil
}

// Open loads the cache from disk
func (c *cache) Open() error {
	c.Lock()
	defer c.Unlock()

	if len(Config.Files.Cache) == 0 {
		return errors.New("cache file path not configured")
	}

	data, err := os.ReadFile(Config.Files.Cache)
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
		logger.Info("Cache expired, reinitializing")
		c.Init()
		return nil
	}

	return nil
}

// Save persists the cache to disk
func (c *cache) Save() error {
	c.Lock()
	defer c.Unlock()

	if len(Config.Files.Cache) == 0 {
		return errors.New("cache file path not configured")
	}

	// Create cache directory if it doesn't exist
	dir := filepath.Dir(Config.Files.Cache)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return errors.Wrap(err, "failed to create cache directory")
	}

	// Marshal cache data
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal cache data")
	}

	// Write to temporary file first
	tmpFile := Config.Files.Cache + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return errors.Wrap(err, "failed to write temporary cache file")
	}

	// Rename temporary file to actual file
	if err := os.Rename(tmpFile, Config.Files.Cache); err != nil {
		os.Remove(tmpFile) // Clean up temp file
		return errors.Wrap(err, "failed to rename temporary cache file")
	}

	return nil
}

// CleanUp removes expired entries from the cache
func (c *cache) CleanUp() {
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

	logger.WithField("expired", expired).Info("Cleaned up cache")
}

// GetStats returns cache statistics
func (c *cache) GetStats() map[string]interface{} {
	c.RLock()
	defer c.RUnlock()

	return map[string]interface{}{
		"hits":      c.stats.Hits,
		"misses":    c.stats.Misses,
		"size":      c.stats.Size,
		"channels":  len(c.Channel),
		"programs":  len(c.Program),
		"metadata":  len(c.Metadata),
		"schedule":  len(c.Schedule),
		"expires":   c.expiration,
	}
}

// Get data from cache
func (c *cache) GetTitle(id, lang string) (t []Title) {

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

func (c *cache) GetSubTitle(id, lang string) (s SubTitle) {

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

func (c *cache) GetDescs(id, subTitle string) (de []Desc) {

	if p, ok := c.Program[id]; ok {

		d := p.Descriptions

		var desc Desc

		for _, tmp := range d.Description1000 {

			switch Config.Options.SubtitleIntoDescription {

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

func (c *cache) GetCredits(id string) (cr Credits) {

	if Config.Options.Credits {

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

func (c *cache) GetCategory(id string) (ca []Category) {

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

func (c *cache) GetEpisodeNum(id string) (ep []EpisodeNum) {

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

func (c *cache) GetPreviouslyShown(id string) (prev *PreviouslyShown) {

	prev = &PreviouslyShown{}

	if p, ok := c.Program[id]; ok {
		prev.Start = p.OriginalAirDate
	}

	return
}

func GetImageUrl(urlid string, token string, name string) {
	url := urlid + "?token=" + token
	filename := Config.Options.ImagesPath + name
	if a, err := os.Stat(filename); err != nil || a.Size() < 500 {
		file, _ := os.Create(filename)
		defer file.Close()
		req, _ := http.Get(url)
		defer req.Body.Close()
		io.Copy(file, req.Body)
		info, _ := os.Stat(filename)
		if info.Size() < 500 {
			log.Println("Max image limit downloaded --skipping image download")
			ImageError = true
			return
		}

	}
}

func (c *cache) GetIcon(id string) (i []Icon) {

	var aspects = []string{"2x3", "4x3", "3x4", "16x9"}
	var uri string
	var width, height int
	var err error
	var nameFinal string
	switch Config.Options.PosterAspect {

	case "all":
		break

	default:
		aspects = []string{Config.Options.PosterAspect}

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
				if Config.Options.TVShowImages && !ImageError {
					GetImageUrl(uri, Token, nameFinal)
				}
				path := "http://" + Config.Options.Hostname + "/images/" + nameFinal
				i = append(i, Icon{Src: path, Height: maxHeight, Width: maxWidth})
			}

		}

	}

	return
}

func (c *cache) GetRating(id, countryCode string) (ra []Rating) {

	if !Config.Options.Rating.Guidelines {
		return
	}

	var add = func(code, body, country string) {

		switch Config.Options.Rating.CountryCodeAsSystem {

		case true:
			ra = append(ra, Rating{Value: code, System: country})

		case false:
			ra = append(ra, Rating{Value: code, System: body})

		}

	}

	/*
	   var prepend = func(code, body, country string) {

	     switch Config.Options.Rating.CountryCodeAsSystem {

	     case true:
	       ra = append([]Rating{{Value: code, System: country}}, ra...)

	     case false:
	       ra = append([]Rating{{Value: code, System: body}}, ra...)

	     }

	   }
	*/

	if p, ok := c.Program[id]; ok {

		switch len(Config.Options.Rating.Countries) {

		case 0:
			for _, r := range p.ContentRating {

				if len(ra) == Config.Options.Rating.MaxEntries && Config.Options.Rating.MaxEntries != 0 {
					return
				}

				if countryCode == r.Country {
					add(r.Code, r.Body, r.Country)
				}

			}

			for _, r := range p.ContentRating {

				if len(ra) == Config.Options.Rating.MaxEntries && Config.Options.Rating.MaxEntries != 0 {
					return
				}

				if countryCode != r.Country {
					add(r.Code, r.Body, r.Country)
				}

			}

		default:
			for _, cCode := range Config.Options.Rating.Countries {

				for _, r := range p.ContentRating {

					if len(ra) == Config.Options.Rating.MaxEntries && Config.Options.Rating.MaxEntries != 0 {
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
