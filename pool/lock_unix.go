//go:build unix

package pool

import (
	"os"
	"syscall"
)

// fileLock is an inter-process advisory lock. On Unix it is built on flock,
// which the kernel releases automatically when the holding process exits or
// crashes, so a crashed renderer never leaves a stale lock behind.
type fileLock struct {
	f    *os.File
	path string
}

func acquireFileLock(dir string) (*fileLock, error) {
	path := dir + "/lock"
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		f.Close()
		if err == syscall.EWOULDBLOCK {
			return nil, ErrLocked
		}
		return nil, err
	}
	return &fileLock{f: f, path: path}, nil
}

func (l *fileLock) release() error {
	if l == nil || l.f == nil {
		return nil
	}
	unlockErr := syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	closeErr := l.f.Close()
	l.f = nil
	if unlockErr != nil {
		return unlockErr
	}
	return closeErr
}
