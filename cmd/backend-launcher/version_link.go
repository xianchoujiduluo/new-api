package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type versionLink struct {
	target     string
	executable string
}

func (launcher *backendLauncher) stableExecutable() string {
	current, err := launcher.resolveVersionLink("current")
	if err != nil {
		launcher.logError("ignore invalid active backend: %v", err)
	}
	if current != nil {
		return current.executable
	}
	return launcher.config.bundledBackend
}

func (launcher *backendLauncher) resolveVersionLink(name string) (*versionLink, error) {
	linkPath := filepath.Join(launcher.config.root, name)
	target, err := os.Readlink(linkPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s link: %w", name, err)
	}
	if filepath.IsAbs(target) {
		return nil, fmt.Errorf("%s link target must be relative", name)
	}

	versionsRoot := filepath.Join(launcher.config.root, "versions")
	executable, err := filepath.EvalSymlinks(filepath.Join(launcher.config.root, target))
	if err != nil {
		return nil, fmt.Errorf("resolve %s link: %w", name, err)
	}
	inside, err := filepath.Rel(versionsRoot, executable)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("%s link points outside the versions directory", name)
	}
	if err := validateExecutable(executable); err != nil {
		return nil, fmt.Errorf("validate %s backend: %w", name, err)
	}
	return &versionLink{target: target, executable: executable}, nil
}

func validateExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return fmt.Errorf("%s is not an executable regular file", path)
	}
	return nil
}

func (launcher *backendLauncher) promoteCandidate(candidate versionLink) error {
	pending, err := launcher.resolveVersionLink("pending")
	if err != nil {
		return err
	}
	if pending == nil || pending.target != candidate.target {
		return fmt.Errorf("pending backend changed during startup")
	}
	current, err := launcher.resolveVersionLink("current")
	if err != nil {
		return err
	}
	if current != nil {
		if err := replaceVersionLink(filepath.Join(launcher.config.root, "previous"), current.target); err != nil {
			return fmt.Errorf("record previous backend: %w", err)
		}
	}
	if err := os.Rename(filepath.Join(launcher.config.root, "pending"), filepath.Join(launcher.config.root, "current")); err != nil {
		return err
	}
	return syncDirectory(launcher.config.root)
}

func replaceVersionLink(linkPath, target string) error {
	temporary, err := os.CreateTemp(filepath.Dir(linkPath), ".backend-link-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return err
	}
	if err := os.Remove(temporaryPath); err != nil {
		return err
	}
	if err := os.Symlink(target, temporaryPath); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, linkPath); err != nil {
		_ = os.Remove(temporaryPath)
		return err
	}
	return nil
}

func (launcher *backendLauncher) discardCandidate(candidate versionLink) {
	pending, err := launcher.resolveVersionLink("pending")
	if err == nil && pending != nil && pending.target == candidate.target {
		if err := os.Remove(filepath.Join(launcher.config.root, "pending")); err != nil && !errors.Is(err, os.ErrNotExist) {
			launcher.logError("remove failed candidate: %v", err)
		}
	}
}

func (launcher *backendLauncher) pendingExists() bool {
	_, err := os.Lstat(filepath.Join(launcher.config.root, "pending"))
	return err == nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
