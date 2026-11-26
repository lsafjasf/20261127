//go:build !unix

package pool

import (
	"os"
	"strconv"
)

// On platforms without flock the lock is an exclusively-created file holding
// the owner PID. It is safe against concurrent opens; if a process crashes
// without calling Close the file must be removed manually. All supported
// render nodes are Unix and use the kernel-released flock instead.
type fileLock struct {
	f    *os.File
	path string
}

func acquireFileLock(dir string) (*fileLock, error) {
	path := dir + "/lock"
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return nil, ErrLocked
		}
		return nil, err
	}
	_, _ = f.WriteString(strconv.Itoa(os.Getpid()))
	return &fileLock{f: f, path: path}, nil
}

func (l *fileLock) release() error {
	if l == nil || l.f == nil {
		return nil
	}
	closeErr := l.f.Close()
	removeErr := os.Remove(l.path)
	l.f = nil
	if closeErr != nil {
		return closeErr
	}
	return removeErr
}
