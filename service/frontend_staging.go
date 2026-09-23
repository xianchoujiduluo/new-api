package service

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/QuantumNous/new-api/common"
)

// frontendStagingMeta marks a staged release on disk. It lives next to the
// staging directory so a staged frontend survives a process restart, which the
// in-process job snapshot cannot.
type frontendStagingMeta struct {
	Source   string `json:"source"`
	StagedAt int64  `json:"staged_at"`
}

// FrontendStagedRelease reports a release extracted but not yet activated.
type FrontendStagedRelease struct {
	Staged   bool   `json:"staged"`
	Source   string `json:"source,omitempty"`
	StagedAt int64  `json:"staged_at,omitempty"`
}

// FrontendStagingDir returns where a staged release is extracted.
func FrontendStagingDir(root string) string {
	if root == "" {
		root = FrontendAssetsDir()
	}
	return filepath.Join(filepath.Dir(root), "staging")
}

func frontendStagingMetaPath(root string) string {
	if root == "" {
		root = FrontendAssetsDir()
	}
	return filepath.Join(filepath.Dir(root), "staging.json")
}

// StagedFrontendRelease reports the staged release recorded on disk.
func StagedFrontendRelease(root string) FrontendStagedRelease {
	if root == "" {
		root = FrontendAssetsDir()
	}
	if info, err := os.Lstat(FrontendStagingDir(root)); err != nil || !info.IsDir() {
		return FrontendStagedRelease{}
	}
	staged := FrontendStagedRelease{Staged: true}
	encoded, err := os.ReadFile(frontendStagingMetaPath(root))
	if err != nil {
		return staged
	}
	meta := frontendStagingMeta{}
	if err := common.Unmarshal(encoded, &meta); err != nil {
		return staged
	}
	staged.Source = meta.Source
	staged.StagedAt = meta.StagedAt
	return staged
}

func writeFrontendStagingMeta(root, source string, stagedAt int64) error {
	encoded, err := common.Marshal(frontendStagingMeta{Source: source, StagedAt: stagedAt})
	if err != nil {
		return fmt.Errorf("记录前端暂存信息失败: %w", err)
	}
	return os.WriteFile(frontendStagingMetaPath(root), encoded, 0644)
}

func removeFrontendStagingMeta(root string) {
	_ = os.Remove(frontendStagingMetaPath(root))
}
