package pool

import (
	"errors"
	"testing"
)

func TestSecondOpenIsRejectedWhileLocked(t *testing.T) {
	dir := t.TempDir()
	first, _, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	second, _, err := Open(dir)
	if !errors.Is(err, ErrLocked) {
		if second != nil {
			second.Close()
		}
		t.Fatalf("expected ErrLocked, got %v", err)
	}

	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	third, _, err := Open(dir)
	if err != nil {
		t.Fatalf("open after release: %v", err)
	}
	defer third.Close()
}
