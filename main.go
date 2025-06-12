package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// AppName : Application name
const AppName = "guide2go"

// Version : Application version
const Version = "1.2.0"

// Config : Global configuration
var Config config
var Config2 string

// logger is the global logger instance
var logger = logrus.New()

func init() {
	// Configure logger
	logger.SetFormatter(&logrus.JSONFormatter{})
	logger.SetOutput(os.Stdout)
	logger.SetLevel(logrus.InfoLevel)
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		logger.Info("Received shutdown signal")
		cancel()
	}()

	var configure = flag.String("configure", "", "Create or modify the configuration file [filename.yaml]")
	var config = flag.String("config", "", "Get data from Schedules Direct with configuration file [filename.yaml]")
	var h = flag.Bool("h", false, "Show help")

	flag.Parse()
	Config2 = *config

	logger.WithFields(logrus.Fields{
		"version": Version,
		"app":     AppName,
	}).Info("Starting application")

	if *h {
		fmt.Println()
		flag.Usage()
		os.Exit(0)
	}

	if len(*configure) != 0 {
		if err := Configure(*configure); err != nil {
			logger.WithError(err).Fatal("Failed to configure application")
		}
		os.Exit(0)
	}

	if len(*config) != 0 {
		var sd SD
		if err := sd.Update(*config); err != nil {
			logger.WithError(err).Fatal("Failed to update data")
		}

		if Config.Options.TVShowImages || Config.Options.ProxyImages {
			if err := Server(ctx); err != nil {
				logger.WithError(err).Fatal("Server error")
			}
		}
	}
}

// ShowErr logs an error with additional context
func ShowErr(err error) {
	logger.WithError(err).Error("Application error")
}
