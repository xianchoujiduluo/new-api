package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type backendLauncher struct {
	config      launcherConfig
	signals     <-chan os.Signal
	stdout      io.Writer
	stderr      io.Writer
	httpClient  *http.Client
	healthCheck func() bool
}

func newBackendLauncher(config launcherConfig, signals <-chan os.Signal, stdout, stderr io.Writer) *backendLauncher {
	launcher := &backendLauncher{
		config:  config,
		signals: signals,
		stdout:  stdout,
		stderr:  stderr,
		httpClient: &http.Client{
			Timeout: 3 * time.Second,
		},
	}
	launcher.healthCheck = launcher.backendHealthy
	return launcher
}

func (launcher *backendLauncher) run(args []string) int {
	if err := os.MkdirAll(filepath.Join(launcher.config.root, "versions"), 0755); err != nil {
		launcher.logError("create backend update directory: %v", err)
		return 1
	}
	if err := validateExecutable(launcher.config.bundledBackend); err != nil {
		launcher.logError("bundled backend is unavailable: %v", err)
		return 1
	}

	for {
		candidate, err := launcher.resolveVersionLink("pending")
		if err != nil {
			launcher.logError("ignore invalid pending backend: %v", err)
			_ = os.Remove(filepath.Join(launcher.config.root, "pending"))
		}
		if candidate != nil {
			if exitCode, stopped := launcher.runCandidate(*candidate, args); stopped {
				return exitCode
			}
			continue
		}

		executable := launcher.stableExecutable()
		exitCode, stopped := launcher.runStable(executable, args)
		if stopped {
			return exitCode
		}
		if launcher.pendingExists() {
			continue
		}
		return exitCode
	}
}

func (launcher *backendLauncher) runCandidate(candidate versionLink, args []string) (int, bool) {
	launcher.log("starting candidate backend %s", candidate.target)
	command, done, err := launcher.start(candidate.executable, args)
	if err != nil {
		launcher.logError("start candidate backend: %v", err)
		launcher.discardCandidate(candidate)
		return 1, false
	}

	healthy, running, exitCode, stopped := launcher.waitForCandidate(command, done)
	if stopped {
		return exitCode, true
	}
	if !healthy {
		launcher.logError("candidate backend failed startup; rolling back")
		if running {
			launcher.stopProcess(command, done)
		}
		launcher.discardCandidate(candidate)
		return exitCode, false
	}
	if err := launcher.promoteCandidate(candidate); err != nil {
		launcher.logError("activate candidate backend: %v", err)
		launcher.stopProcess(command, done)
		launcher.discardCandidate(candidate)
		return 1, false
	}

	launcher.log("candidate backend is healthy and active")
	exitCode, stopped = launcher.waitForExit(command, done)
	return exitCode, stopped
}

func (launcher *backendLauncher) runStable(executable string, args []string) (int, bool) {
	launcher.log("starting backend %s", executable)
	command, done, err := launcher.start(executable, args)
	if err != nil {
		launcher.logError("start backend: %v", err)
		return 1, false
	}
	return launcher.waitForExit(command, done)
}

func (launcher *backendLauncher) start(executable string, args []string) (*exec.Cmd, <-chan error, error) {
	command := exec.Command(executable, args...)
	command.Stdin = os.Stdin
	command.Stdout = launcher.stdout
	command.Stderr = launcher.stderr
	command.Env = withEnvironment(os.Environ(), map[string]string{
		"NEW_API_BACKEND_SUPERVISED": "1",
		"BACKEND_UPDATE_DIR":         launcher.config.root,
	})
	if err := command.Start(); err != nil {
		return nil, nil, err
	}
	done := make(chan error, 1)
	go func() {
		done <- command.Wait()
	}()
	return command, done, nil
}

func (launcher *backendLauncher) waitForCandidate(command *exec.Cmd, done <-chan error) (bool, bool, int, bool) {
	deadline := time.NewTimer(launcher.config.startupTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(launcher.config.healthPollInterval)
	defer ticker.Stop()

	for {
		if launcher.healthCheck() {
			return true, true, 0, false
		}
		select {
		case err := <-done:
			return false, false, processExitCode(err), false
		case signal := <-launcher.signals:
			launcher.forwardSignal(command, signal)
			return false, false, launcher.waitAfterSignal(command, done), true
		case <-deadline.C:
			return false, true, 1, false
		case <-ticker.C:
		}
	}
}

func (launcher *backendLauncher) waitForExit(command *exec.Cmd, done <-chan error) (int, bool) {
	select {
	case err := <-done:
		return processExitCode(err), false
	case signal := <-launcher.signals:
		launcher.forwardSignal(command, signal)
		return launcher.waitAfterSignal(command, done), true
	}
}

func (launcher *backendLauncher) waitAfterSignal(command *exec.Cmd, done <-chan error) int {
	timer := time.NewTimer(launcher.config.stopTimeout)
	defer timer.Stop()
	select {
	case err := <-done:
		return processExitCode(err)
	case <-timer.C:
		launcher.logError("backend did not stop in time; killing it")
		_ = command.Process.Kill()
		return processExitCode(<-done)
	}
}

func (launcher *backendLauncher) stopProcess(command *exec.Cmd, done <-chan error) {
	_ = command.Process.Signal(syscall.SIGTERM)
	timer := time.NewTimer(launcher.config.stopTimeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		_ = command.Process.Kill()
		<-done
	}
}

func (launcher *backendLauncher) forwardSignal(command *exec.Cmd, signal os.Signal) {
	if err := command.Process.Signal(signal); err != nil && !errors.Is(err, os.ErrProcessDone) {
		launcher.logError("forward signal %v: %v", signal, err)
	}
}

func (launcher *backendLauncher) backendHealthy() bool {
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, launcher.config.healthURL, nil)
	if err != nil {
		return false
	}
	response, err := launcher.httpClient.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	return response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices
}

func withEnvironment(environment []string, values map[string]string) []string {
	result := make([]string, 0, len(environment)+len(values))
	for _, entry := range environment {
		name, _, found := strings.Cut(entry, "=")
		if _, replaced := values[name]; found && replaced {
			continue
		}
		result = append(result, entry)
	}
	for name, value := range values {
		result = append(result, name+"="+value)
	}
	return result
}

func processExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		return 1
	}
	if status, ok := exitError.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		return 128 + int(status.Signal())
	}
	if code := exitError.ExitCode(); code >= 0 {
		return code
	}
	return 1
}

func (launcher *backendLauncher) log(format string, values ...any) {
	fmt.Fprintf(launcher.stdout, "backend launcher: "+format+"\n", values...)
}

func (launcher *backendLauncher) logError(format string, values ...any) {
	fmt.Fprintf(launcher.stderr, "backend launcher: "+format+"\n", values...)
}
