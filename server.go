package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/ulule/limiter/v3"
	"github.com/ulule/limiter/v3/drivers/store/memory"
	"github.com/sirupsen/logrus"
)

// Server starts the HTTP server with security headers and timeouts.
func Server(ctx context.Context) error {
	port := strings.Split(Config.Options.Hostname, ":")
	var addr string
	serverImagesPath := Config.Options.ImagesPath
	fs := http.FileServer(http.Dir(serverImagesPath))

	if len(port) == 2 {
		addr = ":" + port[1]
	} else {
		logger.Info("No port found, using port 8080")
		addr = ":8080"
	}

	logger.WithFields(logrus.Fields{
		"addr":         addr,
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
				logger.WithError(err).Error("Rate limiter error")
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
			logger.WithFields(logrus.Fields{
				"method":     r.Method,
				"path":       r.URL.Path,
				"remote_ip":  r.RemoteAddr,
				"user_agent": r.UserAgent(),
				"duration":   time.Since(start),
			}).Info("Request processed")
		})
	})

	if Config.Options.ProxyImages {
		r.HandleFunc("/images/{id}", proxyImages)
	} else if Config.Options.TVShowImages {
		r.PathPrefix("/images/").Handler(http.StripPrefix("/images/", fs))
	}
	r.HandleFunc("/run", run)

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
			logger.WithError(err).Fatal("Server error")
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

func proxyImages(w http.ResponseWriter, r *http.Request) {
	image := mux.Vars(r)
	url := "https://json.schedulesdirect.org/20141201/image/" + image["id"] + "?token=" + Token
	
	logger.WithFields(logrus.Fields{
		"image_id": image["id"],
		"url":      url,
	}).Debug("Proxying image request")

	http.Redirect(w, r, url, http.StatusSeeOther)
}

func run(w http.ResponseWriter, r *http.Request) {
	var sd SD
	go func() {
		if err := sd.Update(Config2); err != nil {
			logger.WithError(err).Error("Failed to update EPG data")
		}
	}()
	fmt.Fprint(w, "Grabbing EPG")
}
