package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	config, err := loadLauncherConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "backend launcher: %v\n", err)
		os.Exit(2)
	}

	signals := make(chan os.Signal, 2)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)

	launcher := newBackendLauncher(config, signals, os.Stdout, os.Stderr)
	os.Exit(launcher.run(os.Args[1:]))
}
