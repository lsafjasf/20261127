package checkpoint

import (
	"context"
	"os"
	"time"
)

type workspaceLock struct {
	file *os.File
}

// Lock acquires an exclusive workspace lock, waiting until ctx is canceled.
// A power loss or process kill releases the OS lock automatically.
func (s *Store) Lock(ctx context.Context) error {
	s.lockMu.Lock()
	defer s.lockMu.Unlock()
	if s.processLock != nil {
		return nil
	}

	lock, err := acquireWorkspaceLock(ctx, s.lockPath)
	if err != nil {
		return err
	}
	s.processLock = lock
	return nil
}

// Unlock releases the workspace lock.
func (s *Store) Unlock() error {
	s.lockMu.Lock()
	defer s.lockMu.Unlock()
	if s.processLock == nil {
		return nil
	}
	err := s.processLock.release()
	s.processLock = nil
	return err
}

func waitLockRetry(ctx context.Context) error {
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
