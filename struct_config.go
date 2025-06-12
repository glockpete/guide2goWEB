package main

import (
	"time"
)

// Config represents the main configuration structure for the application
type config struct {
	File       string   `yaml:"-" json:"-"` // Internal file path
	ChannelIDs []string `yaml:"-" json:"-"` // Internal channel IDs cache

	Account struct {
		Username string `yaml:"Username" json:"username" validate:"required"`
		Password string `yaml:"Password" json:"password" validate:"required"`
	} `yaml:"Account" json:"account"`

	Files struct {
		Cache string `yaml:"Cache" json:"cache" validate:"required"`
		XMLTV string `yaml:"XMLTV" json:"xmltv" validate:"required"`
	} `yaml:"Files" json:"files"`

	Options struct {
		PosterAspect            string        `yaml:"Poster Aspect" json:"poster_aspect" validate:"oneof=portrait landscape square"`
		Schedule                int           `yaml:"Schedule Days" json:"schedule_days" validate:"min=1,max=14"`
		SubtitleIntoDescription bool          `yaml:"Subtitle into Description" json:"subtitle_into_description"`
		Credits                 bool          `yaml:"Insert credits tag into XML file" json:"credits"`
		TVShowImages            bool          `yaml:"Local Images Cache" json:"tv_show_images"`
		ImagesPath              string        `yaml:"Images Path" json:"images_path" validate:"required"`
		ProxyImages             bool          `yaml:"Proxy Images" json:"proxy_images"`
		Hostname                string        `yaml:"Hostname" json:"hostname" validate:"required,hostname_port"`
		CacheExpiration         time.Duration `yaml:"Cache Expiration" json:"cache_expiration" validate:"min=1h,max=168h"` // 1 hour to 1 week

		Rating struct {
			Guidelines          bool     `yaml:"Insert rating tag into XML file" json:"guidelines"`
			MaxEntries          int      `yaml:"Maximum rating entries. 0 for all entries" json:"max_entries" validate:"min=0,max=10"`
			Countries           []string `yaml:"Preferred countries. ISO 3166-1 alpha-3 country code. Leave empty for all systems" json:"countries" validate:"dive,iso3166_1_alpha3"`
			CountryCodeAsSystem bool     `yaml:"Use country code as rating system" json:"country_code_as_system"`
		} `yaml:"Rating" json:"rating"`

		SDDownloadErrors bool `yaml:"Show download errors from Schedules Direct in the log" json:"sd_download_errors"`
	} `yaml:"Options" json:"options"`

	Station []channel `yaml:"Station" json:"station" validate:"dive"`
}

// Channel represents a TV channel configuration
type channel struct {
	Name        string        `yaml:"Name" json:"name" validate:"required"`
	DisplayName []DisplayName `yaml:"-" json:"display_name" xml:"display-name"`
	ID          string        `yaml:"ID" json:"station_id" xml:"id,attr" validate:"required"`
	Lineup      string        `yaml:"Lineup" json:"lineup" validate:"required"`
	Date        []string      `yaml:"-" json:"date"`
	Icon        Icon          `yaml:"-" json:"icon" xml:"icon"`
}

// DisplayName represents a channel's display name in different languages (canonical definition)
type DisplayName struct {
	Lang  string `xml:"lang,attr,omitempty" json:"lang,omitempty"`
	Value string `xml:",chardata" json:"value"`
}

// Icon represents a channel's icon configuration (canonical definition)
type Icon struct {
	Src    string `xml:"src,attr" json:"src"`
	Width  int    `xml:"width,attr,omitempty" json:"width,omitempty"`
	Height int    `xml:"height,attr,omitempty" json:"height,omitempty"`
}
