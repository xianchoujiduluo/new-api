package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/frontend_setting"
)

const (
	frontendVersionFileName   = "version.json"
	frontendManifestMaxBytes  = int64(2 * 1024 * 1024)
	frontendPreviousSuffix    = ".previous"
	frontendRollbackSourceTag = "rollback:"
)

// frontendBuildInfo mirrors the version.json written into every frontend
// build by the release workflow. Older builds without the file simply report
// an empty version, which keeps update checks conservative.
type frontendBuildInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	BuiltAt int64  `json:"built_at"`
}

// FrontendPreviousDir returns the directory holding the build that was live
// before the most recent activation. It exists so an operator can roll back.
func FrontendPreviousDir(root string) string {
	if strings.TrimSpace(root) == "" {
		root = FrontendAssetsDir()
	}
	return filepath.Clean(root) + frontendPreviousSuffix
}

func readFrontendBuildInfo(dir string) frontendBuildInfo {
	if strings.TrimSpace(dir) == "" {
		return frontendBuildInfo{}
	}
	encoded, err := os.ReadFile(filepath.Join(dir, frontendVersionFileName))
	if err != nil {
		return frontendBuildInfo{}
	}
	info := frontendBuildInfo{}
	if err := common.Unmarshal(encoded, &info); err != nil {
		return frontendBuildInfo{}
	}
	return info
}

// FrontendUpdateStatus reports the live, staged and previous frontend builds.
type FrontendUpdateStatus struct {
	Supported       bool   `json:"supported"`
	CurrentVersion  string `json:"current_version"`
	CurrentCommit   string `json:"current_commit"`
	PendingVersion  string `json:"pending_version,omitempty"`
	PreviousVersion string `json:"previous_version,omitempty"`
	Staged          bool   `json:"staged"`
	CanRollback     bool   `json:"can_rollback"`
}

func FrontendUpdateStatusSnapshot(root string) (*FrontendUpdateStatus, error) {
	if strings.TrimSpace(root) == "" {
		root = FrontendAssetsDir()
	}
	current := readFrontendBuildInfo(root)
	status := &FrontendUpdateStatus{
		Supported:      true,
		CurrentVersion: current.Version,
		CurrentCommit:  current.Commit,
	}
	staged := StagedFrontendRelease(root)
	if staged.Staged {
		status.Staged = true
		info := readFrontendBuildInfo(FrontendStagingDir(root))
		status.PendingVersion = info.Version
	}
	previousDir := FrontendPreviousDir(root)
	if info, err := os.Lstat(previousDir); err == nil && info.IsDir() {
		previous := readFrontendBuildInfo(previousDir)
		status.PreviousVersion = previous.Version
		status.CanRollback = true
	}
	return status, nil
}

// ConfiguredFrontendManifestURL returns the persisted manifest URL. The
// default keeps update checks working without extra configuration.
func ConfiguredFrontendManifestURL() string {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	if configured, ok := common.OptionMap[frontend_setting.ManifestURLOptionKey]; ok {
		return strings.TrimSpace(configured)
	}
	return frontend_setting.DefaultManifestURL
}

// frontendManifest is the published file describing the latest frontend build.
type frontendManifest struct {
	SchemaVersion int    `json:"schema_version"`
	Version       string `json:"version"`
	Commit        string `json:"commit"`
	PublishedAt   string `json:"published_at,omitempty"`
}

// FrontendCheckResult compares the live build with the published manifest.
type FrontendCheckResult struct {
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version"`
	LatestCommit    string `json:"latest_commit"`
	PublishedAt     string `json:"published_at,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
}

func CheckConfiguredFrontend(ctx context.Context) (*FrontendCheckResult, error) {
	rawURL := ConfiguredFrontendManifestURL()
	if rawURL == "" {
		return nil, fmt.Errorf("前端更新清单地址未配置")
	}
	if err := frontend_setting.ValidateManifestURL(rawURL); err != nil {
		return nil, err
	}
	manifest, err := fetchFrontendManifest(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(manifest.Version) == "" {
		return nil, fmt.Errorf("前端更新清单缺少版本号")
	}
	status, err := FrontendUpdateStatusSnapshot(FrontendAssetsDir())
	if err != nil {
		return nil, err
	}
	return &FrontendCheckResult{
		CurrentVersion:  status.CurrentVersion,
		LatestVersion:   manifest.Version,
		LatestCommit:    manifest.Commit,
		PublishedAt:     manifest.PublishedAt,
		UpdateAvailable: manifest.Version != status.CurrentVersion,
	}, nil
}

func fetchFrontendManifest(ctx context.Context, rawURL string) (frontendManifest, error) {
	updater := getDefaultFrontendUpdater()
	if err := updater.ValidateFetchURL(rawURL); err != nil {
		return frontendManifest{}, err
	}
	client, err := updater.httpClient()
	if err != nil {
		return frontendManifest{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	fetchCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return frontendManifest{}, fmt.Errorf("创建前端更新清单请求失败: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "new-api-frontend-updater")
	resp, err := client.Do(req)
	if err != nil {
		return frontendManifest{}, fmt.Errorf("获取前端更新清单失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return frontendManifest{}, fmt.Errorf("获取前端更新清单失败: 状态码 %d", resp.StatusCode)
	}
	encoder, err := io.ReadAll(io.LimitReader(resp.Body, frontendManifestMaxBytes))
	if err != nil {
		return frontendManifest{}, fmt.Errorf("读取前端更新清单失败: %w", err)
	}
	manifest := frontendManifest{}
	if err := common.Unmarshal(encoder, &manifest); err != nil {
		return frontendManifest{}, fmt.Errorf("解析前端更新清单失败: %w", err)
	}
	return manifest, nil
}

// StageFrontendRollback moves the retired build back into staging so the
// operator can activate it with the normal activation step. Staging rather than
// swapping keeps both directions of the rollback explicit and auditable.
func StageFrontendRollback() error {
	updater := getDefaultFrontendUpdater()
	previous := FrontendPreviousDir(updater.Root)
	info, err := os.Lstat(previous)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("没有可回滚的前端版本")
		}
		return fmt.Errorf("读取上一前端版本失败: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("上一前端版本不是安全目录: %s", previous)
	}
	staging := FrontendStagingDir(updater.Root)
	if err := os.RemoveAll(staging); err != nil {
		return fmt.Errorf("清理暂存前端目录失败: %w", err)
	}
	if err := os.Rename(previous, staging); err != nil {
		return fmt.Errorf("暂存上一前端版本失败: %w", err)
	}
	version := readFrontendBuildInfo(staging).Version
	if err := writeFrontendStagingMeta(updater.Root, frontendRollbackSourceTag+version, common.GetTimestamp()); err != nil {
		return err
	}
	// The in-process job snapshot may still describe a previous download.
	defaultFrontendDownloadTracker.reset()
	return nil
}
