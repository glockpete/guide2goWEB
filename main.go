package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"github.com/yourusername/guide2go/web/handlers"
)

// AppName : Application name
const AppName = "guide2go"

// Version : Application version
const Version = "1.2.0"

// App holds application-wide dependencies
type App struct {
	Config  config
	Config2 string
	Logger  *logrus.Logger
	Cache   CacheStore
	SD      SchedulesDirectClient
	Token   string
}

func newApp() *App {
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})
	logger.SetOutput(os.Stdout)
	logger.SetLevel(logrus.InfoLevel)
	return &App{
		Logger: logger,
		Cache:  &cache{},
		SD:     &SD{},
	}
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	app := newApp()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		app.Logger.Info("Received shutdown signal")
		cancel()
	}()

	var configure = flag.String("configure", "", "Create or modify the configuration file [filename.yaml]")
	var config = flag.String("config", "", "Get data from Schedules Direct with configuration file [filename.yaml]")
	var webPort = flag.String("web-port", "", "Start web UI on the specified port (e.g. 8080)")
	var h = flag.Bool("h", false, "Show help")

	flag.Parse()
	app.Config2 = *config

	app.Logger.WithFields(logrus.Fields{
		"version": Version,
		"app":     AppName,
	}).Info("Starting application")

	if *h {
		fmt.Println()
		flag.Usage()
		os.Exit(0)
	}

	if *webPort != "" {
		app.StartWebServer(*webPort)
		return
	}

	if len(*configure) != 0 {
		if err := app.Configure(*configure); err != nil {
			app.Logger.WithError(err).Fatal("Failed to configure application")
		}
		os.Exit(0)
	}

	if len(*config) != 0 {
		var sd SD
		if err := app.Update(ctx, &sd, *config); err != nil {
			app.Logger.WithError(err).Fatal("Failed to update data")
		}
		if app.Config.Options.TVShowImages || app.Config.Options.ProxyImages {
			if err := app.Server(ctx); err != nil {
				app.Logger.WithError(err).Fatal("Server error")
			}
		}
	}
}

// ShowErr logs an error with additional context
func (app *App) ShowErr(err error) {
	app.Logger.WithError(err).Error("Application error")
}

// StartWebServer starts the web UI server on the given port
func (app *App) StartWebServer(port string) {
	r := mux.NewRouter()
	handlers.RegisterRoutes(r)
	app.Logger.WithField("port", port).Info("Web UI server started")
	if err := http.ListenAndServe(":"+port, r); err != nil {
		app.Logger.WithError(err).Fatal("Web server error")
	}
}
