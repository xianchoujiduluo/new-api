package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/frontend_setting"
)

const (
	defaultFrontendAssetsDir = "/data/frontend/current"
	frontendArchiveMaxBytes  = int64(256 * 1024 * 1024)
	frontendUpdateTimeout    = 10 * time.Minute
)

// FrontendAssetsDir returns the directory used for a downloaded frontend.
// FRONTEND_ASSETS_DIR is optional and is mainly useful for non-container
// deployments; the Docker layout defaults to /data/frontend/current.
func FrontendAssetsDir() string {
	if configured := strings.TrimSpace(os.Getenv("FRONTEND_ASSETS_DIR")); configured != "" {
		return filepath.Clean(configured)
	}
	return defaultFrontendAssetsDir
}

// FrontendUpdater downloads and atomically activates a frontend archive.
// URLValidator is injectable for tests; production instances use the global
// SSRF-protected URL validator.
type FrontendUpdater struct {
	Root         string
	Client       *http.Client
	URLValidator func(string) error
	// Reporter observes download and extraction progress. It is optional and
	// only set by the asynchronous download endpoint.
	Reporter FrontendDownloadReporter

	mu sync.Mutex
}

func NewFrontendUpdater(root string, client *http.Client) *FrontendUpdater {
	if strings.TrimSpace(root) == "" {
		root = FrontendAssetsDir()
	}
	return &FrontendUpdater{
		Root:         filepath.Clean(root),
		Client:       client,
		URLValidator: ValidateSSRFProtectedFetchURL,
	}
}

var (
	defaultFrontendUpdaterMu sync.Mutex
	defaultFrontendUpdater   *FrontendUpdater
)

func getDefaultFrontendUpdater() *FrontendUpdater {
	defaultFrontendUpdaterMu.Lock()
	defer defaultFrontendUpdaterMu.Unlock()
	if defaultFrontendUpdater == nil {
		defaultFrontendUpdater = NewFrontendUpdater(FrontendAssetsDir(), nil)
	}
	return defaultFrontendUpdater
}

// ConfiguredFrontendDownloadURL returns the current persisted archive URL.
func ConfiguredFrontendDownloadURL() string {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	return common.OptionMap[frontend_setting.DownloadURLOptionKey]
}

// ConfiguredFrontendDownloadProxy returns the optional dedicated proxy for
// frontend archive downloads. An empty value means no dedicated proxy.
func ConfiguredFrontendDownloadProxy() string {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	return common.OptionMap[frontend_setting.DownloadProxyOptionKey]
}

// ExternalFrontendEnabled reports whether the external frontend has been
// configured. Clearing the URL switches traffic back to the embedded build.
func ExternalFrontendEnabled() bool {
	return strings.TrimSpace(ConfiguredFrontendDownloadURL()) != ""
}

// Update downloads, validates, extracts, and activates an archive.
// Update stages the archive and immediately activates it. It is retained for
// callers that want both steps; the management API drives them separately so an
// operator can inspect a staged release before swapping live assets.
func (u *FrontendUpdater) Update(ctx context.Context, rawURL string) error {
	if err := u.Stage(ctx, rawURL); err != nil {
		return err
	}
	return u.Activate()
}

// Stage downloads, validates and extracts a frontend archive into the staging
// directory. The live assets are not touched.
func (u *FrontendUpdater) Stage(ctx context.Context, rawURL string) error {
	if u == nil {
		return fmt.Errorf("前端更新器未初始化")
	}
	rawURL = strings.TrimSpace(rawURL)
	if err := frontend_setting.ValidateDownloadURL(rawURL); err != nil {
		return err
	}
	if rawURL == "" {
		return fmt.Errorf("前端下载地址未配置")
	}
	if u.URLValidator != nil {
		if err := u.URLValidator(rawURL); err != nil {
			return fmt.Errorf("前端下载地址被安全策略拒绝: %w", err)
		}
	}

	u.mu.Lock()
	defer u.mu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	stageCtx, cancel := context.WithTimeout(ctx, frontendUpdateTimeout)
	defer cancel()

	archivePath, err := u.download(stageCtx, rawURL)
	if err != nil {
		return err
	}
	defer os.Remove(archivePath)

	parent := filepath.Dir(u.Root)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return fmt.Errorf("创建前端目录失败: %w", err)
	}
	if info, statErr := os.Lstat(u.Root); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("前端目录不是安全目录: %s", u.Root)
		}
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("检查前端目录失败: %w", statErr)
	}

	tempRoot, err := os.MkdirTemp(parent, ".frontend-stage-")
	if err != nil {
		return fmt.Errorf("创建前端临时目录失败: %w", err)
	}
	keepTemp := false
	defer func() {
		if !keepTemp {
			_ = os.RemoveAll(tempRoot)
		}
	}()

	u.reportStage(FrontendStateExtracting, "正在解压前端压缩包")
	if err := extractFrontendArchive(archivePath, tempRoot); err != nil {
		return err
	}
	if err := normalizeFrontendRoot(tempRoot); err != nil {
		return err
	}

	// Publish into the staging path only after the archive validated, so an
	// interrupted or rejected download never leaves a half-written staging.
	staging := FrontendStagingDir(u.Root)
	if err := os.RemoveAll(staging); err != nil {
		return fmt.Errorf("清理旧暂存前端目录失败: %w", err)
	}
	if err := os.Rename(tempRoot, staging); err != nil {
		return fmt.Errorf("暂存前端目录失败: %w", err)
	}
	keepTemp = true
	if err := writeFrontendStagingMeta(u.Root, rawURL, common.GetTimestamp()); err != nil {
		return err
	}
	return nil
}

// Activate swaps the live assets for the staged release. It is a directory
// rename, so it is effectively instantaneous.
func (u *FrontendUpdater) Activate() error {
	if u == nil {
		return fmt.Errorf("前端更新器未初始化")
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.activateLocked()
}

func (u *FrontendUpdater) activateLocked() error {
	staging := FrontendStagingDir(u.Root)
	if info, err := os.Lstat(staging); err != nil || !info.IsDir() {
		return fmt.Errorf("暂存前端目录不存在")
	}
	if err := activateFrontendDirectory(u.Root, staging); err != nil {
		return err
	}
	removeFrontendStagingMeta(u.Root)
	return nil
}

func (u *FrontendUpdater) reportStage(stage FrontendDownloadState, message string) {
	if u.Reporter == nil {
		return
	}
	u.Reporter.Stage(stage, message)
}

func (u *FrontendUpdater) download(ctx context.Context, rawURL string) (string, error) {
	client := u.Client
	if client == nil {
		if proxyURL := strings.TrimSpace(ConfiguredFrontendDownloadProxy()); proxyURL != "" {
			if err := frontend_setting.ValidateDownloadProxy(proxyURL); err != nil {
				return "", err
			}
			var err error
			client, err = GetHttpClientWithProxy(proxyURL)
			if err != nil {
				return "", fmt.Errorf("创建前端下载代理客户端失败: %w", err)
			}
		} else {
			client = GetSSRFProtectedHTTPClient()
			if client == nil {
				client = &http.Client{}
			}
		}
	}
	if client != nil {
		strictClient := *client
		strictClient.CheckRedirect = checkProtectedFetchRedirect
		client = &strictClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("创建前端下载请求失败: %w", err)
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("User-Agent", "new-api-frontend-updater")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("下载前端压缩包失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("下载前端压缩包失败: HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > frontendArchiveMaxBytes {
		return "", fmt.Errorf("前端压缩包超过大小限制 (%d MB)", frontendArchiveMaxBytes/(1024*1024))
	}

	temp, err := os.CreateTemp("", "new-api-frontend-*.archive")
	if err != nil {
		return "", fmt.Errorf("创建下载临时文件失败: %w", err)
	}
	tempPath := temp.Name()
	removeTemp := true
	defer func() {
		_ = temp.Close()
		if removeTemp {
			_ = os.Remove(tempPath)
		}
	}()
	limited := io.LimitReader(resp.Body, frontendArchiveMaxBytes+1)
	var source io.Reader = limited
	if u.Reporter != nil {
		total := resp.ContentLength
		u.Reporter.Total(total, rawURL)
		source = &progressReader{r: limited, onProgress: func(done int64) {
			u.Reporter.Progress(done, total)
		}}
	}
	n, err := io.Copy(temp, source)
	if err != nil {
		return "", fmt.Errorf("保存前端压缩包失败: %w", err)
	}
	if n > frontendArchiveMaxBytes {
		return "", fmt.Errorf("前端压缩包超过大小限制 (%d MB)", frontendArchiveMaxBytes/(1024*1024))
	}
	if err := temp.Close(); err != nil {
		return "", fmt.Errorf("关闭下载临时文件失败: %w", err)
	}
	removeTemp = false
	return tempPath, nil
}

func activateFrontendDirectory(current, next string) error {
	info, err := os.Lstat(current)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("当前前端目录不是安全目录")
		}
		backup := current + ".previous"
		_ = os.RemoveAll(backup)
		if err := os.Rename(current, backup); err != nil {
			return fmt.Errorf("准备替换前端目录失败: %w", err)
		}
		if err := os.Rename(next, current); err != nil {
			_ = os.Rename(backup, current)
			return fmt.Errorf("激活前端目录失败: %w", err)
		}
		if err := os.RemoveAll(backup); err != nil {
			common.SysError(fmt.Sprintf("清理旧前端目录失败: %v", err))
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("检查当前前端目录失败: %w", err)
	}
	if err := os.Rename(next, current); err != nil {
		return fmt.Errorf("激活前端目录失败: %w", err)
	}
	return nil
}
