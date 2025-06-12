// Package main provides Guide2Go, a tool to generate XMLTV files from Schedules Direct JSON API.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
)

const (
	maxRetries     = 3
	retryDelay     = 2 * time.Second
	maxBackoff     = 30 * time.Second
	requestTimeout = 30 * time.Second
)

var (
	// rateLimiter limits requests to Schedules Direct API
	rateLimiter = rate.NewLimiter(rate.Every(100*time.Millisecond), 1)
)

// SD represents the Schedules Direct API client
type SD struct {
	BaseURL string
	Token   string
	client  *http.Client

	// SD Request
	Req struct {
		URL         string
		Data        []byte
		Type        string
		Compression bool
		Parameter   string
		Call        string
	}

	// SD Response
	Resp struct {
		Body []byte

		// Login
		Login struct {
			Message  string    `json:"message"`
			Code     int       `json:"code"`
			ServerID string    `json:"serverID"`
			Datetime time.Time `json:"datetime"`
			Token    string    `json:"token"`
		}

		// Status
		Status struct {
			Account struct {
				Expires    time.Time     `json:"expires"`
				MaxLineups int64         `json:"maxLineups"`
				Messages   []interface{} `json:"messages"`
			} `json:"account"`
			Code    int    `json:"code"`
			Message string `json:"message"`

			Datetime       string `json:"datetime"`
			LastDataUpdate string `json:"lastDataUpdate"`
			Lineups        []struct {
				Lineup   string `json:"lineup"`
				Modified string `json:"modified"`
				Name     string `json:"name"`
				URI      string `json:"uri"`
			} `json:"lineups"`
			Notifications []interface{} `json:"notifications"`
			ServerID      string        `json:"serverID"`
			SystemStatus  []struct {
				Date    string `json:"date"`
				Message string `json:"message"`
				Status  string `json:"status"`
			} `json:"systemStatus"`
		}

		// Other response fields remain unchanged...
	}

	// SD API Calls
	Login     func() error
	Status    func() error
	Countries func() error
	Headends  func() error
	Lineups   func() error
	Delete    func() error
	Channels  func() error
	Schedule  func() error
	Program   func() error
}

// Init initializes the Schedules Direct client
func (sd *SD) Init() error {
	sd.BaseURL = "https://json.schedulesdirect.org/20141201/"
	sd.client = &http.Client{
		Timeout: requestTimeout,
	}

	sd.Login = func() error {
		sd.Req.URL = sd.BaseURL + "token"
		sd.Req.Type = "POST"
		sd.Req.Call = "login"
		sd.Req.Compression = false
		sd.Token = ""

		login := Config.Account
		data, err := json.MarshalIndent(login, "", "  ")
		if err != nil {
			return errors.Wrap(err, "failed to marshal login data")
		}
		sd.Req.Data = data

		if err := sd.Connect(); err != nil {
			if sd.Resp.Login.Code != 0 {
				return errors.New(sd.Resp.Login.Message)
			}
			return err
		}

		logger.WithFields(logrus.Fields{
			"message": sd.Resp.Login.Message,
		}).Info("Successfully logged in to Schedules Direct")

		sd.Token = sd.Resp.Login.Token
		Token = sd.Token
		return nil
	}

	sd.Status = func() error {
		sd.Req.URL = sd.BaseURL + "status"
		sd.Req.Type = "GET"
		sd.Req.Data = nil
		sd.Req.Call = "status"
		sd.Req.Compression = false

		if err := sd.Connect(); err != nil {
			return err
		}

		logger.WithFields(logrus.Fields{
			"expires":    sd.Resp.Status.Account.Expires,
			"lineups":    len(sd.Resp.Status.Lineups),
			"maxLineups": sd.Resp.Status.Account.MaxLineups,
			"channels":   len(Config.Station),
		}).Info("Schedules Direct status")

		for _, status := range sd.Resp.Status.SystemStatus {
			logger.WithFields(logrus.Fields{
				"status":  status.Status,
				"message": status.Message,
			}).Info("System status")
		}

		return nil
	}

	// Initialize other API methods...
	return nil
}

// Connect sends the HTTP request to Schedules Direct with retries and rate limiting
func (sd *SD) Connect() error {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		// Wait for rate limiter
		if err := rateLimiter.Wait(context.Background()); err != nil {
			return errors.Wrap(err, "rate limiter error")
		}

		// Create request
		req, err := http.NewRequest(sd.Req.Type, sd.Req.URL, bytes.NewBuffer(sd.Req.Data))
		if err != nil {
			return errors.Wrap(err, "failed to create request")
		}

		// Set headers
		if sd.Req.Compression {
			req.Header.Set("Accept-Encoding", "deflate,gzip")
		}
		req.Header.Set("Token", sd.Token)
		req.Header.Set("User-Agent", AppName)
		req.Header.Set("X-Custom-Header", AppName)
		req.Header.Set("Content-Type", "application/json")

		// Send request
		resp, err := sd.client.Do(req)
		if err != nil {
			lastErr = errors.Wrap(err, "request failed")
			time.Sleep(backoff(attempt))
			continue
		}

		// Read response
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = errors.Wrap(err, "failed to read response")
			time.Sleep(backoff(attempt))
			continue
		}

		sd.Resp.Body = body

		// Process response based on call type
		if err := sd.processResponse(); err != nil {
			lastErr = err
			if isRetryableError(err) {
				time.Sleep(backoff(attempt))
				continue
			}
			return err
		}

		return nil
	}

	return errors.Wrap(lastErr, "all retry attempts failed")
}

// processResponse processes the API response based on the call type
func (sd *SD) processResponse() error {
	var sdStatus SDStatus

	switch sd.Req.Call {
	case "login":
		if err := json.Unmarshal(sd.Resp.Body, &sd.Resp.Login); err != nil {
			return errors.Wrap(err, "failed to unmarshal login response")
		}
		sdStatus.Code = sd.Resp.Login.Code
		sdStatus.Message = sd.Resp.Login.Message

	case "status":
		if err := json.Unmarshal(sd.Resp.Body, &sd.Resp.Status); err != nil {
			return errors.Wrap(err, "failed to unmarshal status response")
		}
		sdStatus.Code = sd.Resp.Status.Code
		sdStatus.Message = sd.Resp.Status.Message

	// Add other cases...

	default:
		return errors.New("unknown API call type")
	}

	// Check for API errors
	if sdStatus.Code != 0 {
		return errors.New(sdStatus.Message)
	}

	return nil
}

// backoff calculates exponential backoff duration
func backoff(attempt int) time.Duration {
	duration := retryDelay * time.Duration(1<<uint(attempt))
	if duration > maxBackoff {
		duration = maxBackoff
	}
	return duration
}

// isRetryableError determines if an error should trigger a retry
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check for network errors
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}

	// Check for HTTP errors
	var httpErr *http.Response
	if errors.As(err, &httpErr) {
		switch httpErr.StatusCode {
		case http.StatusTooManyRequests,
			http.StatusInternalServerError,
			http.StatusBadGateway,
			http.StatusServiceUnavailable,
			http.StatusGatewayTimeout:
			return true
		}
	}

	return false
}
