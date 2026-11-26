//go:build unix

package checkpoint

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
)

func acquireWorkspaceLock(ctx context.Context, path string) (*workspaceLock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}

	for {
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return &workspaceLock{file: file}, nil
		}
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			if waitErr := waitLockRetry(ctx); waitErr != nil {
				_ = file.Close()
				return nil, waitErr
			}
			continue
		}
		_ = file.Close()
		return nil, fmt.Errorf("acquire file lock: %w", err)
	}
}

func (l *workspaceLock) release() error {
	if l == nil || l.file == nil {
		return nil
	}
	unlockErr := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	closeErr := l.file.Close()
	return errors.Join(unlockErr, closeErr)
}
