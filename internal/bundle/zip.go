package bundle

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// writeZip 将暂存目录打包为 zip，内部使用临时文件避免半成品归档。
func writeZip(zipPath, stage string, force bool) (string, error) {
	abs, err := filepath.Abs(zipPath)
	if err != nil {
		return "", fmt.Errorf("zip 路径非法: %w", err)
	}
	if _, err := os.Stat(abs); err == nil && !force {
		return "", fmt.Errorf("zip 已存在（使用 --force 覆盖）: %s", abs)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", fmt.Errorf("创建 zip 父目录失败: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(abs), ".videopack-zip-*.tmp")
	if err != nil {
		return "", fmt.Errorf("创建临时 zip 失败: %w", err)
	}
	tmpName := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmpName)
		}
	}()

	zw := zip.NewWriter(tmp)
	walkErr := filepath.Walk(stage, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(stage, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		name := filepath.ToSlash(rel)
		if info.IsDir() {
			name = strings.TrimSuffix(name, "/") + "/"
			if _, err := zw.CreateHeader(&zip.FileHeader{
				Name: name, Method: zip.Store,
			}); err != nil {
				return err
			}
			return nil
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = name
		header.Method = zip.Deflate
		writer, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		src, err := os.Open(p)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(writer, src)
		closeErr := src.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if walkErr != nil {
		_ = zw.Close()
		_ = tmp.Close()
		return "", fmt.Errorf("写入 zip 失败: %w", walkErr)
	}
	if err := zw.Close(); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("完成 zip 失败: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("同步 zip 失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("关闭 zip 失败: %w", err)
	}
	if force {
		_ = os.Remove(abs)
	}
	if err := os.Rename(tmpName, abs); err != nil {
		return "", fmt.Errorf("提交 zip 失败: %w", err)
	}
	committed = true
	return abs, nil
}
