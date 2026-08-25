package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBackendRoot        = "/data/backend"
	defaultBundledBackend     = "/usr/local/lib/new-api/new-api"
	defaultStartupTimeout     = 120 * time.Second
	defaultHealthPollInterval = time.Second
	defaultStopTimeout        = 130 * time.Second
)

type launcherConfig struct {
	root               string
	bundledBackend     string
	healthURL          string
	startupTimeout     time.Duration
	healthPollInterval time.Duration
	stopTimeout        time.Duration
}

func loadLauncherConfig() (launcherConfig, error) {
	root := envOrDefault("BACKEND_UPDATE_DIR", defaultBackendRoot)
	bundled := envOrDefault("BACKEND_BUNDLED_EXECUTABLE", defaultBundledBackend)
	port := envOrDefault("PORT", "3000")
	healthURL := envOrDefault("BACKEND_HEALTH_URL", "http://127.0.0.1:"+port+"/api/status")

	startupTimeout, err := durationFromSeconds("BACKEND_STARTUP_TIMEOUT_SECONDS", defaultStartupTimeout)
	if err != nil {
		return launcherConfig{}, err
	}
	stopTimeout, err := durationFromSeconds("BACKEND_STOP_TIMEOUT_SECONDS", defaultStopTimeout)
	if err != nil {
		return launcherConfig{}, err
	}

	return launcherConfig{
		root:               filepath.Clean(root),
		bundledBackend:     filepath.Clean(bundled),
		healthURL:          healthURL,
		startupTimeout:     startupTimeout,
		healthPollInterval: defaultHealthPollInterval,
		stopTimeout:        stopTimeout,
	}, nil
}

func durationFromSeconds(name string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return time.Duration(seconds) * time.Second, nil
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
