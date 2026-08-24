package service

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/frontend_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFrontendUpdaterActivatesTarGzAndRemovesPreviousFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "current")
	previousAsset := filepath.Join(root, "old.js")
	require.NoError(t, os.MkdirAll(root, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "index.html"), []byte("old"), 0644))
	require.NoError(t, os.WriteFile(previousAsset, []byte("old"), 0644))

	archive := tarGzArchive(t, map[string]string{
		"index.html":       "new",
		"assets/app.js":    "console.log('new')",
		"assets/style.css": "body{}",
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	updater := NewFrontendUpdater(root, server.Client())
	updater.URLValidator = func(string) error { return nil }
	require.NoError(t, updater.Update(context.Background(), server.URL+"/frontend.tar.gz"))

	index, err := os.ReadFile(filepath.Join(root, "index.html"))
	require.NoError(t, err)
	assert.Equal(t, "new", string(index))
	assert.FileExists(t, filepath.Join(root, "assets", "app.js"))
	assert.NoFileExists(t, previousAsset)
}

func TestFrontendUpdaterActivatesZipWithNestedReleaseDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "current")
	archive := zipArchive(t, map[string]string{
		"release/index.html": "zip index",
		"release/app.js":     "zip app",
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	updater := NewFrontendUpdater(root, server.Client())
	updater.URLValidator = func(string) error { return nil }
	require.NoError(t, updater.Update(context.Background(), server.URL))
	content, err := os.ReadFile(filepath.Join(root, "index.html"))
	require.NoError(t, err)
	assert.Equal(t, "zip index", string(content))
}

func TestFrontendUpdaterRejectsPathTraversalAndKeepsCurrentVersion(t *testing.T) {
	root := filepath.Join(t.TempDir(), "current")
	require.NoError(t, os.MkdirAll(root, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "index.html"), []byte("stable"), 0644))
	escapePath := filepath.Join(filepath.Dir(root), "escape.txt")
	_ = os.Remove(escapePath)

	archive := tarGzArchive(t, map[string]string{
		"../escape.txt": "must not be written",
		"index.html":    "malicious",
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	updater := NewFrontendUpdater(root, server.Client())
	updater.URLValidator = func(string) error { return nil }
	err := updater.Update(context.Background(), server.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "路径穿越")
	content, readErr := os.ReadFile(filepath.Join(root, "index.html"))
	require.NoError(t, readErr)
	assert.Equal(t, "stable", string(content))
	assert.NoFileExists(t, escapePath)
}

func tarGzArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, content := range files {
		data := []byte(content)
		require.NoError(t, tarWriter.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0644,
			Size: int64(len(data)),
		}))
		_, err := tarWriter.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, tarWriter.Close())
	require.NoError(t, gzipWriter.Close())
	return buffer.Bytes()
}

func zipArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	zipWriter := zip.NewWriter(&buffer)
	for name, content := range files {
		entry, err := zipWriter.Create(name)
		require.NoError(t, err)
		_, err = io.Copy(entry, bytes.NewBufferString(content))
		require.NoError(t, err)
	}
	require.NoError(t, zipWriter.Close())
	return buffer.Bytes()
}

func TestFrontendUpdaterRejectsUnsupportedURL(t *testing.T) {
	updater := NewFrontendUpdater(filepath.Join(t.TempDir(), "current"), nil)
	err := updater.Update(context.Background(), "file:///tmp/frontend.tar.gz")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "http 或 https")
}

func TestExternalFrontendEnabledFollowsConfiguredURL(t *testing.T) {
	previousOptions := common.OptionMap
	common.OptionMapRWMutex.Lock()
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
	})

	assert.False(t, ExternalFrontendEnabled())
	common.OptionMapRWMutex.Lock()
	common.OptionMap[frontend_setting.DownloadURLOptionKey] = "https://example.com/frontend.tar.gz"
	common.OptionMap[frontend_setting.DownloadProxyOptionKey] = "socks5://proxy.example:1080"
	common.OptionMapRWMutex.Unlock()
	assert.Equal(t, "https://example.com/frontend.tar.gz", ConfiguredFrontendDownloadURL())
	assert.Equal(t, "socks5://proxy.example:1080", ConfiguredFrontendDownloadProxy())
	assert.True(t, ExternalFrontendEnabled())
}

func TestFrontendUpdaterUsesConfiguredDownloadProxy(t *testing.T) {
	root := filepath.Join(t.TempDir(), "current")
	archive := tarGzArchive(t, map[string]string{"index.html": "proxied"})
	var proxyRequests int
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyRequests++
		assert.Equal(t, "http", r.URL.Scheme)
		assert.Equal(t, "frontend.invalid", r.URL.Host)
		_, _ = w.Write(archive)
	}))
	defer proxyServer.Close()

	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	previousProxy, hadPreviousProxy := common.OptionMap[frontend_setting.DownloadProxyOptionKey]
	common.OptionMap[frontend_setting.DownloadProxyOptionKey] = proxyServer.URL
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		if hadPreviousProxy {
			common.OptionMap[frontend_setting.DownloadProxyOptionKey] = previousProxy
		} else {
			delete(common.OptionMap, frontend_setting.DownloadProxyOptionKey)
		}
		common.OptionMapRWMutex.Unlock()
	})

	updater := NewFrontendUpdater(root, nil)
	updater.URLValidator = func(string) error { return nil }
	require.NoError(t, updater.Update(context.Background(), "http://frontend.invalid/frontend.tar.gz"))
	assert.Equal(t, 1, proxyRequests)
	content, err := os.ReadFile(filepath.Join(root, "index.html"))
	require.NoError(t, err)
	assert.Equal(t, "proxied", string(content))
}

func TestFrontendUpdaterRejectsInvalidConfiguredDownloadProxy(t *testing.T) {
	root := filepath.Join(t.TempDir(), "current")
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	previousProxy, hadPreviousProxy := common.OptionMap[frontend_setting.DownloadProxyOptionKey]
	common.OptionMap[frontend_setting.DownloadProxyOptionKey] = "ftp://proxy.example:8080"
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		if hadPreviousProxy {
			common.OptionMap[frontend_setting.DownloadProxyOptionKey] = previousProxy
		} else {
			delete(common.OptionMap, frontend_setting.DownloadProxyOptionKey)
		}
		common.OptionMapRWMutex.Unlock()
	})

	updater := NewFrontendUpdater(root, nil)
	updater.URLValidator = func(string) error { return nil }
	err := updater.Update(context.Background(), "http://frontend.invalid/frontend.tar.gz")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "前端下载代理无效")
}
