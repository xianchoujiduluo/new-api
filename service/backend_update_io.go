package service

import (
	"context"
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/setting/backend_setting"
)

func (u *BackendUpdater) fetch(ctx context.Context, rawURL string, limit int64, label string) ([]byte, error) {
	if u.URLValidator != nil {
		if err := u.URLValidator(rawURL); err != nil {
			return nil, fmt.Errorf("%s地址被安全策略拒绝: %w", label, err)
		}
	}
	client, err := u.downloadClient()
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建%s下载请求失败: %w", label, err)
	}
	request.Header.Set("Accept", "application/octet-stream")
	request.Header.Set("User-Agent", "new-api-backend-updater")
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("下载%s失败: %w", label, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("下载%s失败: HTTP %d", label, response.StatusCode)
	}
	if response.ContentLength > limit {
		return nil, fmt.Errorf("%s超过大小限制", label)
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("读取%s失败: %w", label, err)
	}
	if int64(len(content)) > limit {
		return nil, fmt.Errorf("%s超过大小限制", label)
	}
	return content, nil
}

func (u *BackendUpdater) downloadClient() (*http.Client, error) {
	if u.Client != nil {
		return strictBackendDownloadClient(u.Client), nil
	}
	proxyURL := strings.TrimSpace(ConfiguredBackendDownloadProxy())
	if proxyURL != "" {
		if err := backend_setting.ValidateDownloadProxy(proxyURL); err != nil {
			return nil, err
		}
		client, err := GetHttpClientWithProxy(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("创建后端下载代理客户端失败: %w", err)
		}
		return strictBackendDownloadClient(client), nil
	}
	client := GetSSRFProtectedHTTPClient()
	if client == nil {
		client = http.DefaultClient
	}
	return strictBackendDownloadClient(client), nil
}

func strictBackendDownloadClient(client *http.Client) *http.Client {
	if client == nil {
		return http.DefaultClient
	}
	copy := *client
	copy.CheckRedirect = checkProtectedFetchRedirect
	return &copy
}

func (u *BackendUpdater) install(ctx context.Context, manifest BackendManifest, manifestBytes []byte, artifact BackendArtifact, rawURL string) (bool, error) {
	versions := filepath.Join(u.Root, "versions")
	if err := ensureBackendDirectory(u.Root); err != nil {
		return false, err
	}
	if err := ensureBackendDirectory(versions); err != nil {
		return false, err
	}
	staging, err := os.MkdirTemp(versions, ".backend-update-")
	if err != nil {
		return false, fmt.Errorf("创建后端更新临时目录失败: %w", err)
	}
	keepStaging := false
	defer func() {
		if !keepStaging {
			_ = os.RemoveAll(staging)
		}
	}()

	candidate := filepath.Join(staging, "new-api")
	if err := u.downloadBinary(ctx, rawURL, candidate, artifact); err != nil {
		return false, err
	}
	u.reportStage(BackendDownloadVerifying)
	validator := u.CandidateValidator
	if validator == nil {
		validator = validateBackendCandidate
	}
	if err := validator(ctx, candidate, manifest); err != nil {
		return false, err
	}
	u.reportStage(BackendDownloadInstalling)
	if err := os.WriteFile(filepath.Join(staging, "manifest.json"), manifestBytes, 0644); err != nil {
		return false, fmt.Errorf("保存后端更新清单失败: %w", err)
	}

	releaseID := manifest.Version + "-" + strings.ToLower(manifest.Commit)
	destination := filepath.Join(versions, releaseID)
	if err := moveBackendRelease(staging, destination, artifact); err != nil {
		return false, err
	}
	keepStaging = true
	return stageBackendRelease(u.Root, filepath.ToSlash(filepath.Join("versions", releaseID, "new-api")))
}

func (u *BackendUpdater) downloadBinary(ctx context.Context, rawURL, destination string, artifact BackendArtifact) error {
	if u.URLValidator != nil {
		if err := u.URLValidator(rawURL); err != nil {
			return fmt.Errorf("后端更新制品地址被安全策略拒绝: %w", err)
		}
	}
	client, err := u.downloadClient()
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("创建后端更新制品下载请求失败: %w", err)
	}
	request.Header.Set("Accept", "application/octet-stream")
	request.Header.Set("User-Agent", "new-api-backend-updater")
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("下载后端更新制品失败: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("下载后端更新制品失败: HTTP %d", response.StatusCode)
	}
	if response.ContentLength > artifact.Size {
		return fmt.Errorf("后端更新制品超过清单声明的大小")
	}
	return writeBackendCandidate(response.Body, destination, artifact, func(done int64) {
		u.reportProgress(done, artifact.Size)
	})
}

// progressReader counts bytes as they stream through so the download path can
// report progress without buffering the artifact.
type progressReader struct {
	r          io.Reader
	done       int64
	onProgress func(int64)
}

func (p *progressReader) Read(buffer []byte) (int, error) {
	read, err := p.r.Read(buffer)
	if read > 0 {
		p.done += int64(read)
		p.onProgress(p.done)
	}
	return read, err
}

func writeBackendCandidate(source io.Reader, destination string, artifact BackendArtifact, onProgress func(int64)) error {
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0755)
	if err != nil {
		return fmt.Errorf("创建后端候选文件失败: %w", err)
	}
	hasher := sha256.New()
	reader := io.LimitReader(source, artifact.Size+1)
	if onProgress != nil {
		reader = &progressReader{r: reader, onProgress: onProgress}
	}
	written, copyErr := io.Copy(io.MultiWriter(output, hasher), reader)
	chmodErr := output.Chmod(0755)
	syncErr := output.Sync()
	closeErr := output.Close()
	if copyErr != nil {
		return fmt.Errorf("保存后端更新制品失败: %w", copyErr)
	}
	if chmodErr != nil || syncErr != nil || closeErr != nil {
		return fmt.Errorf("同步后端更新制品失败")
	}
	if written != artifact.Size {
		return fmt.Errorf("后端更新制品大小校验失败")
	}
	actualDigest := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actualDigest, artifact.SHA256) {
		return fmt.Errorf("后端更新制品 SHA-256 校验失败")
	}
	return nil
}

func ensureBackendDirectory(path string) error {
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("后端更新目录不是安全目录: %s", path)
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("检查后端更新目录失败: %w", err)
	}
	if err := os.MkdirAll(path, 0755); err != nil {
		return fmt.Errorf("创建后端更新目录失败: %w", err)
	}
	return nil
}

func moveBackendRelease(staging, destination string, artifact BackendArtifact) error {
	if info, err := os.Lstat(destination); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("目标后端版本路径不是安全目录")
		}
		matches, hashErr := backendFileMatches(filepath.Join(destination, "new-api"), artifact)
		if hashErr != nil || !matches {
			return fmt.Errorf("目标后端版本目录已存在但制品不匹配")
		}
		if err := os.RemoveAll(staging); err != nil {
			return fmt.Errorf("清理后端更新临时目录失败: %w", err)
		}
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("检查目标后端版本目录失败: %w", err)
	}
	if err := os.Rename(staging, destination); err != nil {
		return fmt.Errorf("安装后端更新制品失败: %w", err)
	}
	if err := syncBackendDirectory(filepath.Dir(destination)); err != nil {
		return fmt.Errorf("同步后端版本目录失败: %w", err)
	}
	return nil
}

func backendFileMatches(path string, artifact BackendArtifact) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return false, fmt.Errorf("后端版本制品不是安全的普通文件")
	}
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()
	hasher := sha256.New()
	written, err := io.Copy(hasher, io.LimitReader(file, artifact.Size+1))
	if err != nil || written != artifact.Size {
		return false, err
	}
	return strings.EqualFold(hex.EncodeToString(hasher.Sum(nil)), artifact.SHA256), nil
}

func validateBackendCandidate(ctx context.Context, path string, manifest BackendManifest) error {
	file, err := elf.Open(path)
	if err != nil {
		return fmt.Errorf("后端候选文件不是有效的 ELF 可执行文件: %w", err)
	}
	machine := file.Machine
	_ = file.Close()
	wantMachine := elf.EM_X86_64
	if runtimeArch := backendRuntimeArch(); runtimeArch == "arm64" {
		wantMachine = elf.EM_AARCH64
	}
	if machine != wantMachine {
		return fmt.Errorf("后端候选文件架构不匹配")
	}
	versionCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	command := exec.CommandContext(versionCtx, path, "--version")
	command.Env = environmentWithoutVersion(os.Environ())
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("执行后端候选版本检查失败: %w", err)
	}
	if strings.TrimSpace(string(output)) != manifest.Version {
		return fmt.Errorf("后端候选版本与更新清单不匹配")
	}
	return nil
}

func backendRuntimeArch() string {
	return runtime.GOARCH
}

func environmentWithoutVersion(environment []string) []string {
	filtered := make([]string, 0, len(environment))
	for _, entry := range environment {
		if !strings.HasPrefix(entry, "VERSION=") {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func backendSignatureURL(manifestURL string) (string, error) {
	parsed, err := url.Parse(manifestURL)
	if err != nil {
		return "", err
	}
	parsed.Path += ".sig"
	return parsed.String(), nil
}
