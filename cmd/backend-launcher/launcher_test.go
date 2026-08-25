package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLauncherRollsBackFailedCandidateAndPassesArguments(t *testing.T) {
	root := t.TempDir()
	versions := filepath.Join(root, "versions", "failed")
	require.NoError(t, os.MkdirAll(versions, 0755))
	writeExecutable(t, filepath.Join(versions, "new-api"), "#!/bin/sh\nexit 9\n")
	require.NoError(t, os.Symlink("versions/failed/new-api", filepath.Join(root, "pending")))

	argumentsFile := filepath.Join(t.TempDir(), "arguments")
	bundled := filepath.Join(t.TempDir(), "bundled")
	writeExecutable(t, bundled, "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$LAUNCHER_TEST_ARGUMENTS\"\n")
	t.Setenv("LAUNCHER_TEST_ARGUMENTS", argumentsFile)

	launcher := testLauncher(root, bundled, "http://127.0.0.1:1/unavailable")
	assert.Equal(t, 0, launcher.run([]string{"--log-dir", "/app/logs"}))
	assert.NoFileExists(t, filepath.Join(root, "pending"))
	assert.Equal(t, "--log-dir\n/app/logs\n", readFile(t, argumentsFile))
}

func TestLauncherPromotesHealthyCandidate(t *testing.T) {
	root := t.TempDir()
	stableVersions := filepath.Join(root, "versions", "stable")
	versions := filepath.Join(root, "versions", "healthy")
	require.NoError(t, os.MkdirAll(stableVersions, 0755))
	require.NoError(t, os.MkdirAll(versions, 0755))
	writeExecutable(t, filepath.Join(stableVersions, "new-api"), "#!/bin/sh\nexit 0\n")
	writeExecutable(t, filepath.Join(versions, "new-api"), "#!/bin/sh\nsleep 0.2\n")
	require.NoError(t, os.Symlink("versions/stable/new-api", filepath.Join(root, "current")))
	require.NoError(t, os.Symlink("versions/healthy/new-api", filepath.Join(root, "pending")))

	bundled := filepath.Join(t.TempDir(), "bundled")
	writeExecutable(t, bundled, "#!/bin/sh\nexit 1\n")
	launcher := testLauncher(root, bundled, "http://127.0.0.1:1/unavailable")
	launcher.healthCheck = func() bool { return true }

	assert.Equal(t, 0, launcher.run(nil))
	assert.NoFileExists(t, filepath.Join(root, "pending"))
	target, err := os.Readlink(filepath.Join(root, "current"))
	require.NoError(t, err)
	assert.Equal(t, "versions/healthy/new-api", target)
	previous, err := os.Readlink(filepath.Join(root, "previous"))
	require.NoError(t, err)
	assert.Equal(t, "versions/stable/new-api", previous)
}

func TestResolveVersionLinkRejectsTargetOutsideVersions(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "versions"), 0755))
	executable := filepath.Join(root, "outside")
	writeExecutable(t, executable, "#!/bin/sh\nexit 0\n")
	require.NoError(t, os.Symlink("outside", filepath.Join(root, "pending")))

	launcher := testLauncher(root, executable, "http://127.0.0.1:1/unavailable")
	resolved, err := launcher.resolveVersionLink("pending")
	require.Error(t, err)
	assert.Nil(t, resolved)
	assert.Contains(t, err.Error(), "outside the versions directory")
}

func TestWithEnvironmentReplacesLauncherValues(t *testing.T) {
	environment := withEnvironment(
		[]string{"PATH=/bin", "BACKEND_UPDATE_DIR=/old", "NEW_API_BACKEND_SUPERVISED=0"},
		map[string]string{
			"BACKEND_UPDATE_DIR":         "/new",
			"NEW_API_BACKEND_SUPERVISED": "1",
		},
	)

	assert.Contains(t, environment, "PATH=/bin")
	assert.Contains(t, environment, "BACKEND_UPDATE_DIR=/new")
	assert.Contains(t, environment, "NEW_API_BACKEND_SUPERVISED=1")
	assert.NotContains(t, environment, "BACKEND_UPDATE_DIR=/old")
	assert.NotContains(t, environment, "NEW_API_BACKEND_SUPERVISED=0")
}

func testLauncher(root, bundled, healthURL string) *backendLauncher {
	launcher := newBackendLauncher(launcherConfig{
		root:               root,
		bundledBackend:     bundled,
		healthURL:          healthURL,
		startupTimeout:     100 * time.Millisecond,
		healthPollInterval: 5 * time.Millisecond,
		stopTimeout:        100 * time.Millisecond,
	}, make(chan os.Signal), io.Discard, io.Discard)
	launcher.httpClient.Timeout = 10 * time.Millisecond
	return launcher
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0755))
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return strings.ReplaceAll(string(content), "\r\n", "\n")
}
