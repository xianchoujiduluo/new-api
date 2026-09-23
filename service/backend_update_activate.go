package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func stageBackendRelease(root, target string) (bool, error) {
	current, err := readBackendReleaseLink(root, filepath.Join(root, "current"))
	if err != nil {
		return false, err
	}
	if current == target {
		return false, nil
	}
	pendingPath := filepath.Join(root, "pending")
	pending, err := readBackendReleaseLink(root, pendingPath)
	if err != nil {
		return false, err
	}
	if pending == target {
		return true, nil
	}
	if err := replaceBackendSymlink(pendingPath, target); err != nil {
		return false, fmt.Errorf("暂存后端候选版本失败: %w", err)
	}
	if err := syncBackendDirectory(root); err != nil {
		return false, fmt.Errorf("同步后端候选版本失败: %w", err)
	}
	return true, nil
}

func readBackendReleaseLink(root, linkPath string) (string, error) {
	target, err := os.Readlink(linkPath)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("后端版本指针不是符号链接: %s", linkPath)
	}
	if filepath.IsAbs(target) {
		return "", fmt.Errorf("后端版本指针包含绝对路径")
	}
	clean := filepath.ToSlash(filepath.Clean(target))
	if !strings.HasPrefix(clean, "versions/") || strings.Contains(clean, "../") {
		return "", fmt.Errorf("后端版本指针超出版本目录")
	}
	resolved := filepath.Join(root, filepath.FromSlash(clean))
	relative, err := filepath.Rel(filepath.Join(root, "versions"), resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("后端版本指针超出版本目录")
	}
	return clean, nil
}

func replaceBackendSymlink(linkPath, target string) error {
	parent := filepath.Dir(linkPath)
	temp, err := os.CreateTemp(parent, ".backend-link-")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	if closeErr := temp.Close(); closeErr != nil {
		_ = os.Remove(tempPath)
		return closeErr
	}
	if err := os.Remove(tempPath); err != nil {
		return err
	}
	if err := os.Symlink(target, tempPath); err != nil {
		return err
	}
	if err := os.Rename(tempPath, linkPath); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return nil
}

func syncBackendDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

// DiscardPendingBackend removes a staged release that has not been activated
// yet, so an accidental download can be dropped without restarting. The running
// `current` release is never touched.
func DiscardPendingBackend(root string) (bool, error) {
	if strings.TrimSpace(root) == "" {
		root = BackendUpdateDir()
	}
	pendingPath := filepath.Join(root, "pending")
	if _, err := os.Lstat(pendingPath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("读取待生效后端失败: %w", err)
	}
	if err := os.Remove(pendingPath); err != nil {
		return false, fmt.Errorf("丢弃待生效后端失败: %w", err)
	}
	if err := syncBackendDirectory(root); err != nil {
		return false, fmt.Errorf("同步后端更新目录失败: %w", err)
	}
	return true, nil
}
