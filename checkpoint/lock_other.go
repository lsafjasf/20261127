//go:build !unix

package checkpoint

import (
	"context"
	"fmt"
	"os"
)

func acquireWorkspaceLock(ctx context.Context, path string) (*workspaceLock, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	return &workspaceLock{file: file}, nil
}

func (l *workspaceLock) release() error {
	if l == nil || l.file == nil {
		return nil
	}
	return l.file.Close()
}
