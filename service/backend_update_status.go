package service

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

type BackendCheckResult struct {
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version"`
	LatestCommit    string `json:"latest_commit"`
	PublishedAt     string `json:"published_at,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
}

type BackendUpdateStatus struct {
	Supported       bool   `json:"supported"`
	Supervised      bool   `json:"supervised"`
	CurrentVersion  string `json:"current_version"`
	CurrentCommit   string `json:"current_commit"`
	PendingVersion  string `json:"pending_version,omitempty"`
	PreviousVersion string `json:"previous_version,omitempty"`
	RestartRequired bool   `json:"restart_required"`
	CanRollback     bool   `json:"can_rollback"`
}

func BackendUpdateStatusSnapshot(root string) (*BackendUpdateStatus, error) {
	if strings.TrimSpace(root) == "" {
		root = BackendUpdateDir()
	}
	currentVersion := common.BuildVersion
	currentCommit := common.BuildCommit
	current, err := backendReleaseForLink(root, "current")
	if err != nil {
		return nil, err
	}
	if current != nil {
		currentVersion = current.Version
		currentCommit = current.Commit
	}
	pending, err := backendReleaseForLink(root, "pending")
	if err != nil {
		return nil, err
	}
	previous, err := backendReleaseForLink(root, "previous")
	if err != nil {
		return nil, err
	}
	status := &BackendUpdateStatus{
		Supported:       runtime.GOOS == "linux",
		Supervised:      os.Getenv("NEW_API_BACKEND_SUPERVISED") == "1",
		CurrentVersion:  currentVersion,
		CurrentCommit:   currentCommit,
		RestartRequired: pending != nil,
		CanRollback:     previous != nil,
	}
	if pending != nil {
		status.PendingVersion = pending.Version
	}
	if previous != nil {
		status.PreviousVersion = previous.Version
	}
	return status, nil
}

func StageBackendRollback(root string) (*BackendUpdateResult, error) {
	if runtime.GOOS != "linux" || os.Getenv("NEW_API_BACKEND_SUPERVISED") != "1" {
		return nil, fmt.Errorf("当前进程未由后端启动器托管，无法安全回滚")
	}
	if strings.TrimSpace(root) == "" {
		root = BackendUpdateDir()
	}
	previousTarget, err := readBackendReleaseLink(root, filepath.Join(root, "previous"))
	if err != nil {
		return nil, err
	}
	if previousTarget == "" {
		return nil, fmt.Errorf("没有可回滚的后端版本")
	}
	previous, err := backendReleaseForTarget(root, previousTarget)
	if err != nil {
		return nil, err
	}
	if err := replaceBackendSymlink(filepath.Join(root, "pending"), previousTarget); err != nil {
		return nil, fmt.Errorf("暂存后端回滚版本失败: %w", err)
	}
	if err := syncBackendDirectory(root); err != nil {
		return nil, fmt.Errorf("同步后端回滚状态失败: %w", err)
	}
	return &BackendUpdateResult{Version: previous.Version, Commit: previous.Commit, Changed: true}, nil
}

func backendReleaseForLink(root, name string) (*BackendManifest, error) {
	target, err := readBackendReleaseLink(root, filepath.Join(root, name))
	if err != nil || target == "" {
		return nil, err
	}
	return backendReleaseForTarget(root, target)
}

func backendReleaseForTarget(root, target string) (*BackendManifest, error) {
	manifestPath := filepath.Join(filepath.Dir(filepath.Join(root, filepath.FromSlash(target))), "manifest.json")
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("读取后端版本清单失败: %w", err)
	}
	if int64(len(content)) > backendManifestMaxBytes {
		return nil, fmt.Errorf("后端版本清单超过大小限制")
	}
	var manifest BackendManifest
	if err := common.Unmarshal(content, &manifest); err != nil {
		return nil, fmt.Errorf("解析后端版本清单失败: %w", err)
	}
	if !backendReleasePartPattern.MatchString(manifest.Version) || !backendCommitPattern.MatchString(manifest.Commit) {
		return nil, fmt.Errorf("后端版本清单无效")
	}
	return &manifest, nil
}
