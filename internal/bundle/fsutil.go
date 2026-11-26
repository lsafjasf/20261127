package bundle

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

var clockNow = func() time.Time { return time.Now().UTC() }

// copyTrack 将源文件复制到暂存目录并计算 SHA-256，返回清单条目。
func copyTrack(stage string, t Track) (FileEntry, error) {
	dst := filepath.Join(stage, filepath.FromSlash(t.RelPath))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return FileEntry{}, fmt.Errorf("创建目录失败 %s: %w", t.RelPath, err)
	}

	src, err := os.Open(t.SourceAbs)
	if err != nil {
		return FileEntry{}, fmt.Errorf("打开源文件失败 %s: %w", t.SourceAbs, err)
	}
	defer src.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return FileEntry{}, fmt.Errorf("创建目标文件失败 %s: %w", t.RelPath, err)
	}
	hasher := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(out, hasher), src)
	syncErr := out.Sync()
	closeErr := out.Close()
	if copyErr != nil {
		return FileEntry{}, fmt.Errorf("复制文件失败 %s: %w", t.RelPath, copyErr)
	}
	if syncErr != nil {
		return FileEntry{}, fmt.Errorf("落盘文件失败 %s: %w", t.RelPath, syncErr)
	}
	if closeErr != nil {
		return FileEntry{}, fmt.Errorf("关闭文件失败 %s: %w", t.RelPath, closeErr)
	}
	if info, err := os.Stat(t.SourceAbs); err == nil && info.Size() != written {
		return FileEntry{}, fmt.Errorf("复制大小不一致 %s", t.RelPath)
	}

	sum := hex.EncodeToString(hasher.Sum(nil))
	return FileEntry{Path: t.RelPath, Kind: t.Kind, SHA256: sum, Size: written}, nil
}

// syncDir 对目录执行 fsync，保证重命名与新建条目持久化。
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("打开目录失败 %s: %w", dir, err)
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		return fmt.Errorf("同步目录失败 %s: %w", dir, err)
	}
	return nil
}
