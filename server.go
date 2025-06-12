package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/ulule/limiter/v3"
	"github.com/ulule/limiter/v3/drivers/store/memory"
)

var requestCount uint64
var errorCount uint64

// Server starts the HTTP server with security headers and timeouts.
func (app *App) Server(ctx context.Context) error {
	port := strings.Split(app.Config.Options.Hostname, ":")
	var addr string
	serverImagesPath := app.Config.Options.ImagesPath
	fs := http.FileServer(http.Dir(serverImagesPath))

	if len(port) == 2 {
		addr = ":" + port[1]
	} else {
		app.Logger.Info("No port found, using port 8080")
		addr = ":8080"
	}

	app.Logger.WithFields(logrus.Fields{
		"addr":        addr,
		"images_path": serverImagesPath,
	}).Info("Starting server")

	// Create a new rate limiter
	rate := limiter.Rate{
		Period: 1 * time.Minute,
		Limit:  60,
	}
	store := memory.NewStore()
	limiter := limiter.New(store, rate)

	r := mux.NewRouter()

	// Add security middleware
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Security headers
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("X-XSS-Protection", "1; mode=block")
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			w.Header().Set("Content-Security-Policy", "default-src 'self'")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			// CORS headers
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			// Rate limiting
			context, err := limiter.Get(r.Context(), r.RemoteAddr)
			if err != nil {
				app.Logger.WithError(err).Error("Rate limiter error")
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			if context.Reached {
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	})

	// Add request logging middleware
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			next.ServeHTTP(w, r)
			app.Logger.WithFields(logrus.Fields{
				"method":     r.Method,
				"path":       r.URL.Path,
				"remote_ip":  r.RemoteAddr,
				"user_agent": r.UserAgent(),
				"duration":   time.Since(start),
			}).Info("Request processed")
		})
	})

	if app.Config.Options.ProxyImages {
		r.HandleFunc("/images/{id}", app.proxyImages)
	} else if app.Config.Options.TVShowImages {
		r.PathPrefix("/images/").Handler(http.StripPrefix("/images/", fs))
	}
	r.HandleFunc("/run", app.run)
	r.HandleFunc("/health", app.healthCheck)
	r.HandleFunc("/metrics", app.metricsHandler)

	// Add timeouts
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			app.Logger.WithError(err).Fatal("Server error")
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	// Create shutdown context with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return errors.Wrap(err, "server shutdown error")
	}

	return nil
}

// validateImagePath ensures the image path is within the allowed directory and safe
func validateImagePath(basePath, name string) error {
	cleanPath := filepath.Clean(filepath.Join(basePath, name))
	if !strings.HasPrefix(cleanPath, filepath.Clean(basePath)) {
		return fmt.Errorf("invalid image path: %s", cleanPath)
	}
	return nil
}

// isValidImageID checks if the image ID contains only safe characters
var imageIDPattern = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

func isValidImageID(id string) bool {
	return imageIDPattern.MatchString(id)
}

func (app *App) proxyImages(w http.ResponseWriter, r *http.Request) {
	image := mux.Vars(r)
	id := image["id"]
	if !isValidImageID(id) {
		http.Error(w, "Invalid image ID", http.StatusBadRequest)
		return
	}
	url := "https://json.schedulesdirect.org/20141201/image/" + id
	app.Logger.WithFields(logrus.Fields{
		"image_id": id,
		"url":      url,
	}).Debug("Proxying image request")

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		http.Error(w, "Failed to create request", http.StatusInternalServerError)
		return
	}
	req.Header.Set("Authorization", "Bearer "+app.Token)
	resp, err := httpClient.Do(req)
	if err != nil {
		http.Error(w, "Failed to fetch image", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for k, v := range resp.Header {
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (app *App) run(w http.ResponseWriter, r *http.Request) {
	var sd SD
	go func() {
		if err := sd.Update(app.Config2); err != nil {
			app.Logger.WithError(err).Error("Failed to update EPG data")
		}
	}()
	fmt.Fprint(w, "Grabbing EPG")
}

func (app *App) healthCheck(w http.ResponseWriter, r *http.Request) {
	resp := map[string]interface{}{
		"status":  "healthy",
		"version": Version,
	}
	app.Logger.WithField("endpoint", "/health").Info("Health check requested")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (app *App) metricsHandler(w http.ResponseWriter, r *http.Request) {
	atomic.AddUint64(&requestCount, 1)
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "# HELP guide2go_requests_total Total HTTP requests\n")
	fmt.Fprintf(w, "# TYPE guide2go_requests_total counter\n")
	fmt.Fprintf(w, "guide2go_requests_total %d\n", atomic.LoadUint64(&requestCount))
	fmt.Fprintf(w, "# HELP guide2go_errors_total Total HTTP errors\n")
	fmt.Fprintf(w, "# TYPE guide2go_errors_total counter\n")
	fmt.Fprintf(w, "guide2go_errors_total %d\n", atomic.LoadUint64(&errorCount))
	app.Logger.WithField("endpoint", "/metrics").Info("Metrics requested")
}
