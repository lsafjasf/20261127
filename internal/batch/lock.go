package batch

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// ErrLocked means another engine process already owns the output directory.
var ErrLocked = errors.New("output directory is locked by another process")

// Lock is an exclusive, kernel-released advisory lock on the output
// directory. It prevents two renderers from concurrently appending to the
// same WAL. The kernel releases it automatically if a process dies, so no
// stale-lock cleanup is ever needed.
type Lock struct {
	file *os.File
}

// AcquireLock takes an exclusive non-blocking flock on <dir>/lock.
func AcquireLock(dir string) (*Lock, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrLocked
		}
		return nil, err
	}
	return &Lock{file: f}, nil
}

// Release unlocks and closes the lock file.
func (l *Lock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	err := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	if closeErr := l.file.Close(); err == nil {
		err = closeErr
	}
	l.file = nil
	return err
}
