package service

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	frontendExpandedMaxBytes = int64(512 * 1024 * 1024)
	frontendMaxFileBytes     = int64(128 * 1024 * 1024)
	frontendMaxFiles         = 20_000
)

func extractFrontendArchive(archivePath, destination string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("打开前端压缩包失败: %w", err)
	}
	defer file.Close()
	header := make([]byte, 4)
	if _, err := io.ReadFull(file, header); err != nil {
		return fmt.Errorf("读取前端压缩包失败: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("定位前端压缩包失败: %w", err)
	}
	switch {
	case header[0] == 'P' && header[1] == 'K':
		return extractFrontendZip(archivePath, destination)
	case header[0] == 0x1f && header[1] == 0x8b:
		gzipReader, err := gzip.NewReader(file)
		if err != nil {
			return fmt.Errorf("读取 gzip 前端压缩包失败: %w", err)
		}
		defer gzipReader.Close()
		return extractFrontendTar(gzipReader, destination)
	default:
		return extractFrontendTar(file, destination)
	}
}

func extractFrontendTar(reader io.Reader, destination string) error {
	archive := tar.NewReader(reader)
	var total int64
	var files int
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("读取前端 tar 归档失败: %w", err)
		}
		target, err := safeArchivePath(destination, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return fmt.Errorf("创建前端目录失败: %w", err)
			}
		case tar.TypeReg, tar.TypeRegA:
			files++
			if files > frontendMaxFiles {
				return fmt.Errorf("前端归档文件数量超过限制")
			}
			if header.Size < 0 || header.Size > frontendMaxFileBytes || total > frontendExpandedMaxBytes-header.Size {
				return fmt.Errorf("前端归档展开后超过大小限制")
			}
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("创建前端父目录失败: %w", err)
			}
			output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC|os.O_EXCL, sanitizedFileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("创建前端文件失败: %w", err)
			}
			written, copyErr := io.CopyN(output, archive, header.Size)
			closeErr := output.Close()
			if copyErr != nil {
				return fmt.Errorf("解压前端文件失败: %w", copyErr)
			}
			if closeErr != nil {
				return fmt.Errorf("关闭前端文件失败: %w", closeErr)
			}
			if written != header.Size {
				return fmt.Errorf("前端文件大小校验失败")
			}
			total += written
		default:
			return fmt.Errorf("前端归档包含不支持的文件类型: %s", header.Name)
		}
	}
	return nil
}

func extractFrontendZip(archivePath, destination string) error {
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("读取前端 zip 归档失败: %w", err)
	}
	defer archive.Close()
	var total int64
	files := 0
	for _, entry := range archive.File {
		target, err := safeArchivePath(destination, entry.Name)
		if err != nil {
			return err
		}
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return fmt.Errorf("创建前端目录失败: %w", err)
			}
			continue
		}
		if entry.Mode()&os.ModeSymlink != 0 || !entry.Mode().IsRegular() {
			return fmt.Errorf("前端归档包含不支持的文件类型: %s", entry.Name)
		}
		files++
		if files > frontendMaxFiles || entry.UncompressedSize64 > uint64(frontendMaxFileBytes) || total > frontendExpandedMaxBytes-int64(entry.UncompressedSize64) {
			return fmt.Errorf("前端归档展开后超过大小限制")
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return fmt.Errorf("创建前端父目录失败: %w", err)
		}
		input, err := entry.Open()
		if err != nil {
			return fmt.Errorf("打开前端归档文件失败: %w", err)
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC|os.O_EXCL, sanitizedFileMode(int64(entry.Mode())))
		if err != nil {
			input.Close()
			return fmt.Errorf("创建前端文件失败: %w", err)
		}
		written, copyErr := io.CopyN(output, input, int64(entry.UncompressedSize64))
		closeInputErr := input.Close()
		closeOutputErr := output.Close()
		if copyErr != nil {
			return fmt.Errorf("解压前端文件失败: %w", copyErr)
		}
		if closeInputErr != nil || closeOutputErr != nil || written != int64(entry.UncompressedSize64) {
			return fmt.Errorf("前端文件大小校验失败")
		}
		total += written
	}
	return nil
}

func safeArchivePath(root, name string) (string, error) {
	if strings.TrimSpace(name) == "" || strings.ContainsRune(name, 0) || strings.Contains(name, "\\") {
		return "", fmt.Errorf("前端归档包含不安全路径: %q", name)
	}
	name = filepath.ToSlash(name)
	if strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("前端归档包含绝对路径: %q", name)
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("前端归档包含路径穿越: %q", name)
	}
	target := filepath.Join(root, filepath.FromSlash(clean))
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("前端归档包含路径穿越: %q", name)
	}
	return target, nil
}

func sanitizedFileMode(mode int64) os.FileMode {
	if mode&0111 != 0 {
		return 0755
	}
	return 0644
}

func normalizeFrontendRoot(root string) error {
	if fileExists(filepath.Join(root, "index.html")) {
		return nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("检查前端入口文件失败: %w", err)
	}
	if len(entries) == 1 && entries[0].IsDir() && fileExists(filepath.Join(root, entries[0].Name(), "index.html")) {
		promoted := root + ".promoted"
		if err := os.Rename(filepath.Join(root, entries[0].Name()), promoted); err != nil {
			return fmt.Errorf("整理前端归档目录失败: %w", err)
		}
		if err := os.RemoveAll(root); err != nil {
			_ = os.Rename(promoted, filepath.Join(root, entries[0].Name()))
			return fmt.Errorf("整理前端归档目录失败: %w", err)
		}
		if err := os.Rename(promoted, root); err != nil {
			return fmt.Errorf("整理前端归档目录失败: %w", err)
		}
		return nil
	}
	return fmt.Errorf("前端归档缺少 index.html")
}

func fileExists(name string) bool {
	info, err := os.Stat(name)
	return err == nil && info.Mode().IsRegular()
}
