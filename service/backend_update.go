package service

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/backend_setting"
)

const (
	defaultBackendUpdateDir       = "/data/backend"
	backendManifestMaxBytes       = int64(1024 * 1024)
	backendSignatureMaxBytes      = int64(4096)
	backendBinaryMaxBytes         = int64(256 * 1024 * 1024)
	backendUpdateTimeout          = 10 * time.Minute
	defaultBackendUpdatePublicKey = `-----BEGIN PUBLIC KEY-----
MCowBQYDK2VwAyEA8DxuvAeXP2JBMlFhIk1LKwItiAVygCCDe8sCMMeBTDU=
-----END PUBLIC KEY-----`
)

var (
	backendReleasePartPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,127}$`)
	backendCommitPattern      = regexp.MustCompile(`^[0-9a-fA-F]{12,64}$`)
	backendRestartRequests    = make(chan struct{}, 1)
	defaultBackendUpdaterMu   sync.Mutex
	defaultBackendUpdater     *BackendUpdater
)

type BackendArtifact struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type BackendManifest struct {
	SchemaVersion int                        `json:"schema_version"`
	Version       string                     `json:"version"`
	Commit        string                     `json:"commit"`
	PublishedAt   string                     `json:"published_at"`
	Artifacts     map[string]BackendArtifact `json:"artifacts"`
}

type BackendUpdateResult struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Changed bool   `json:"changed"`
}

type BackendUpdater struct {
	Root               string
	Client             *http.Client
	URLValidator       func(string) error
	PublicKey          ed25519.PublicKey
	GOOS               string
	GOARCH             string
	Supervised         bool
	CandidateValidator func(context.Context, string, BackendManifest) error
	// Reporter observes coarse download progress. It is optional and only set by
	// the asynchronous download endpoint; the synchronous path leaves it nil.
	Reporter BackendDownloadReporter

	mu sync.Mutex
}

func BackendUpdateDir() string {
	if configured := strings.TrimSpace(os.Getenv("BACKEND_UPDATE_DIR")); configured != "" {
		return filepath.Clean(configured)
	}
	return defaultBackendUpdateDir
}

func NewBackendUpdater(root string, client *http.Client) *BackendUpdater {
	if strings.TrimSpace(root) == "" {
		root = BackendUpdateDir()
	}
	return &BackendUpdater{
		Root:               filepath.Clean(root),
		Client:             client,
		URLValidator:       ValidateSSRFProtectedFetchURL,
		GOOS:               runtime.GOOS,
		GOARCH:             runtime.GOARCH,
		Supervised:         os.Getenv("NEW_API_BACKEND_SUPERVISED") == "1",
		CandidateValidator: validateBackendCandidate,
	}
}

func ConfiguredBackendManifestURL() string {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	return common.OptionMap[backend_setting.ManifestURLOptionKey]
}

func ConfiguredBackendDownloadProxy() string {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	return common.OptionMap[backend_setting.DownloadProxyOptionKey]
}

func configuredBackendPublicKey() (ed25519.PublicKey, error) {
	raw := strings.TrimSpace(os.Getenv("BACKEND_UPDATE_PUBLIC_KEY"))
	if raw == "" {
		raw = defaultBackendUpdatePublicKey
	}
	if block, _ := pem.Decode([]byte(raw)); block != nil {
		parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("解析后端更新公钥失败: %w", err)
		}
		publicKey, ok := parsed.(ed25519.PublicKey)
		if !ok {
			return nil, fmt.Errorf("后端更新公钥不是 Ed25519 公钥")
		}
		return publicKey, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(decoded) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("后端更新公钥格式无效")
	}
	return ed25519.PublicKey(decoded), nil
}

func UpdateConfiguredBackend(ctx context.Context) (*BackendUpdateResult, error) {
	rawURL := strings.TrimSpace(ConfiguredBackendManifestURL())
	if rawURL == "" {
		return nil, fmt.Errorf("后端更新清单地址未配置")
	}
	return getDefaultBackendUpdater().Update(ctx, rawURL)
}

func CheckConfiguredBackend(ctx context.Context) (*BackendCheckResult, error) {
	rawURL := strings.TrimSpace(ConfiguredBackendManifestURL())
	if rawURL == "" {
		return nil, fmt.Errorf("后端更新清单地址未配置")
	}
	return getDefaultBackendUpdater().Check(ctx, rawURL)
}

func getDefaultBackendUpdater() *BackendUpdater {
	defaultBackendUpdaterMu.Lock()
	defer defaultBackendUpdaterMu.Unlock()
	if defaultBackendUpdater == nil {
		defaultBackendUpdater = NewBackendUpdater(BackendUpdateDir(), nil)
	}
	return defaultBackendUpdater
}

func (u *BackendUpdater) Check(ctx context.Context, manifestURL string) (*BackendCheckResult, error) {
	if u == nil {
		return nil, fmt.Errorf("后端更新器未初始化")
	}
	if u.GOOS != "linux" {
		return nil, fmt.Errorf("后端在线更新仅支持 Linux")
	}
	manifestURL = strings.TrimSpace(manifestURL)
	if err := backend_setting.ValidateManifestURL(manifestURL); err != nil {
		return nil, err
	}
	if manifestURL == "" {
		return nil, fmt.Errorf("后端更新清单地址未配置")
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	checkCtx, cancel := context.WithTimeout(ctx, backendUpdateTimeout)
	defer cancel()
	manifest, _, _, _, err := u.loadManifest(checkCtx, manifestURL)
	if err != nil {
		return nil, err
	}
	status, err := BackendUpdateStatusSnapshot(u.Root)
	if err != nil {
		return nil, err
	}
	updateAvailable := manifest.Version != status.CurrentVersion
	if !updateAvailable && status.CurrentCommit != "" && status.CurrentCommit != "unknown" {
		updateAvailable = manifest.Commit != status.CurrentCommit
	}
	return &BackendCheckResult{
		CurrentVersion:  status.CurrentVersion,
		LatestVersion:   manifest.Version,
		LatestCommit:    manifest.Commit,
		PublishedAt:     manifest.PublishedAt,
		UpdateAvailable: updateAvailable,
	}, nil
}

func (u *BackendUpdater) Update(ctx context.Context, manifestURL string) (*BackendUpdateResult, error) {
	if u == nil {
		return nil, fmt.Errorf("后端更新器未初始化")
	}
	if u.GOOS != "linux" {
		return nil, fmt.Errorf("后端在线更新仅支持 Linux")
	}
	if !u.Supervised {
		return nil, fmt.Errorf("当前进程未由后端启动器托管，无法安全重启")
	}
	manifestURL = strings.TrimSpace(manifestURL)
	if err := backend_setting.ValidateManifestURL(manifestURL); err != nil {
		return nil, err
	}
	if manifestURL == "" {
		return nil, fmt.Errorf("后端更新清单地址未配置")
	}

	u.mu.Lock()
	defer u.mu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	updateCtx, cancel := context.WithTimeout(ctx, backendUpdateTimeout)
	defer cancel()

	manifest, manifestBytes, artifact, artifactURL, err := u.loadManifest(updateCtx, manifestURL)
	if err != nil {
		return nil, err
	}
	u.reportTarget(manifest.Version, manifest.Commit, artifact.Size)
	changed, err := u.install(updateCtx, manifest, manifestBytes, artifact, artifactURL)
	if err != nil {
		return nil, err
	}
	return &BackendUpdateResult{Version: manifest.Version, Commit: manifest.Commit, Changed: changed}, nil
}

func (u *BackendUpdater) reportTarget(version, commit string, totalBytes int64) {
	if u.Reporter == nil {
		return
	}
	u.Reporter.Target(version, commit, totalBytes)
}

func (u *BackendUpdater) reportProgress(doneBytes, totalBytes int64) {
	if u.Reporter == nil {
		return
	}
	u.Reporter.Progress(doneBytes, totalBytes)
}

func (u *BackendUpdater) reportStage(stage BackendDownloadState) {
	if u.Reporter == nil {
		return
	}
	u.Reporter.Stage(stage, backendDownloadProgressMessage(stage))
}

func (u *BackendUpdater) loadManifest(ctx context.Context, rawURL string) (BackendManifest, []byte, BackendArtifact, string, error) {
	var manifest BackendManifest
	manifestBytes, err := u.fetch(ctx, rawURL, backendManifestMaxBytes, "后端更新清单")
	if err != nil {
		return manifest, nil, BackendArtifact{}, "", err
	}
	signatureURL, err := backendSignatureURL(rawURL)
	if err != nil {
		return manifest, nil, BackendArtifact{}, "", fmt.Errorf("解析后端更新清单签名地址失败: %w", err)
	}
	signatureBytes, err := u.fetch(ctx, signatureURL, backendSignatureMaxBytes, "后端更新清单签名")
	if err != nil {
		return manifest, nil, BackendArtifact{}, "", err
	}
	if err := u.verifyManifest(manifestBytes, signatureBytes); err != nil {
		return manifest, nil, BackendArtifact{}, "", err
	}
	if err := common.Unmarshal(manifestBytes, &manifest); err != nil {
		return manifest, nil, BackendArtifact{}, "", fmt.Errorf("解析后端更新清单失败: %w", err)
	}
	artifact, err := u.validateManifest(manifest)
	if err != nil {
		return manifest, nil, BackendArtifact{}, "", err
	}
	artifactURL, err := resolveBackendArtifactURL(rawURL, artifact.URL)
	if err != nil {
		return manifest, nil, BackendArtifact{}, "", err
	}
	return manifest, manifestBytes, artifact, artifactURL, nil
}

func (u *BackendUpdater) verifyManifest(manifest, encodedSignature []byte) error {
	publicKey := u.PublicKey
	if len(publicKey) == 0 {
		var err error
		publicKey, err = configuredBackendPublicKey()
		if err != nil {
			return err
		}
	}
	signature, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(encodedSignature)))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("后端更新清单签名格式无效")
	}
	if !ed25519.Verify(publicKey, manifest, signature) {
		return fmt.Errorf("后端更新清单签名校验失败")
	}
	return nil
}

func (u *BackendUpdater) validateManifest(manifest BackendManifest) (BackendArtifact, error) {
	if manifest.SchemaVersion != 1 {
		return BackendArtifact{}, fmt.Errorf("不支持的后端更新清单版本: %d", manifest.SchemaVersion)
	}
	if !backendReleasePartPattern.MatchString(manifest.Version) || !backendCommitPattern.MatchString(manifest.Commit) {
		return BackendArtifact{}, fmt.Errorf("后端更新清单的版本信息无效")
	}
	artifact, ok := backendArtifactForPlatform(manifest.Artifacts, u.GOOS, u.GOARCH)
	if !ok {
		return BackendArtifact{}, fmt.Errorf("后端更新清单不支持当前平台: %s/%s", u.GOOS, u.GOARCH)
	}
	if (artifact.OS != "" && artifact.OS != u.GOOS) || (artifact.Arch != "" && artifact.Arch != u.GOARCH) {
		return BackendArtifact{}, fmt.Errorf("后端更新制品平台与当前系统不匹配")
	}
	if artifact.Size <= 0 || artifact.Size > backendBinaryMaxBytes {
		return BackendArtifact{}, fmt.Errorf("后端更新制品大小无效")
	}
	digest, err := hex.DecodeString(artifact.SHA256)
	if err != nil || len(digest) != 32 {
		return BackendArtifact{}, fmt.Errorf("后端更新制品 SHA-256 无效")
	}
	if strings.TrimSpace(artifact.URL) == "" {
		return BackendArtifact{}, fmt.Errorf("后端更新制品地址为空")
	}
	return artifact, nil
}

func backendArtifactForPlatform(artifacts map[string]BackendArtifact, goos, goarch string) (BackendArtifact, bool) {
	for _, key := range []string{goos + "-" + goarch, goos + "/" + goarch, goos + "_" + goarch} {
		if artifact, ok := artifacts[key]; ok {
			return artifact, true
		}
	}
	for _, artifact := range artifacts {
		if artifact.OS == goos && artifact.Arch == goarch {
			return artifact, true
		}
	}
	return BackendArtifact{}, false
}

func resolveBackendArtifactURL(manifestURL, artifactURL string) (string, error) {
	base, err := url.Parse(manifestURL)
	if err != nil {
		return "", fmt.Errorf("解析后端更新清单地址失败: %w", err)
	}
	reference, err := url.Parse(strings.TrimSpace(artifactURL))
	if err != nil {
		return "", fmt.Errorf("解析后端更新制品地址失败: %w", err)
	}
	resolved := base.ResolveReference(reference).String()
	if err := backend_setting.ValidateManifestURL(resolved); err != nil {
		return "", fmt.Errorf("后端更新制品地址无效: %w", err)
	}
	return resolved, nil
}

func RequestProcessRestart() {
	select {
	case backendRestartRequests <- struct{}{}:
	default:
	}
}

func ProcessRestartRequests() <-chan struct{} {
	return backendRestartRequests
}
