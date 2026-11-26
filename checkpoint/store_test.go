package checkpoint

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type failAfterReader struct {
	payload []byte
	read    bool
}

func (r *failAfterReader) Read(p []byte) (int, error) {
	if r.read {
		return 0, errors.New("render source disappeared")
	}
	r.read = true
	n := copy(p, r.payload)
	return n, errors.New("render source disappeared")
}

func TestPutReaderFailureQuarantinesPartialOutput(t *testing.T) {
	store := newTestStore(t)
	_, err := store.PutReader(context.Background(), &failAfterReader{payload: []byte("partial-bytes")}, "application/octet-stream")
	if err == nil {
		t.Fatal("PutReader succeeded, want source error")
	}

	stagingEntries, err := os.ReadDir(store.stagingDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(stagingEntries) != 0 {
		t.Fatalf("staging has %d entries, want zero", len(stagingEntries))
	}

	quarantineEntries, err := os.ReadDir(store.quarantineDir)
	if err != nil {
		t.Fatal(err)
	}
	var dataFiles []string
	for _, entry := range quarantineEntries {
		if strings.HasSuffix(entry.Name(), ".part") {
			dataFiles = append(dataFiles, entry.Name())
		}
	}
	if len(dataFiles) != 1 {
		t.Fatalf("quarantine data files = %v, want one partial .part", dataFiles)
	}
	content, err := os.ReadFile(filepath.Join(store.quarantineDir, dataFiles[0]))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "partial-bytes" {
		t.Fatalf("quarantined content = %q, want partial-bytes", content)
	}
}

func TestExportReaderPublishesAtomicallyAndRejectsEscape(t *testing.T) {
	store := newTestStore(t)
	path, err := store.ExportReader(context.Background(), "book/final.txt", strings.NewReader("complete output"))
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "complete output" {
		t.Fatalf("exported content = %q", content)
	}
	if entries, _ := os.ReadDir(store.stagingDir); len(entries) != 0 {
		t.Fatalf("staging entries remain after export: %d", len(entries))
	}
	if _, err := store.ExportReader(context.Background(), "../escape.txt", strings.NewReader("bad")); err == nil {
		t.Fatal("path traversal export succeeded")
	}
	if _, err := os.Stat(filepath.Join(store.Root(), "escape.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("escaped output exists or stat error = %v", err)
	}
}

func TestCheckpointSaveAndLoad(t *testing.T) {
	store := newTestStore(t)
	now := nowUTC()
	input := &Checkpoint{
		JobID:     "job-1",
		CreatedAt: now,
		Nodes: map[string]*NodeRecord{
			"chapter-1": {ID: "chapter-1", Status: StatusCompleted, CacheKey: "key-1"},
		},
	}
	if err := store.SaveCheckpoint(input); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadCheckpoint("job-1")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Nodes["chapter-1"].CacheKey != "key-1" {
		t.Fatalf("loaded checkpoint = %+v", loaded.Nodes["chapter-1"])
	}
}

type blockingReader struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func newBlockingReader() *blockingReader {
	return &blockingReader{started: make(chan struct{}), release: make(chan struct{})}
}

func (r *blockingReader) Read(p []byte) (int, error) {
	r.once.Do(func() { close(r.started) })
	<-r.release
	return 0, io.EOF
}

func TestPutReaderCancelQuarantinesOpenPartial(t *testing.T) {
	store := newTestStore(t)
	reader := newBlockingReader()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := store.PutReader(ctx, reader, "application/octet-stream")
		done <- err
	}()

	select {
	case <-reader.started:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for read to start")
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("PutReader error = %v, want context.Canceled", err)
	}
	close(reader.release)

	entries, err := os.ReadDir(store.stagingDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("staging has %d entries after cancel", len(entries))
	}
	quarantineEntries, err := os.ReadDir(store.quarantineDir)
	if err != nil {
		t.Fatal(err)
	}
	foundPart := false
	for _, entry := range quarantineEntries {
		if strings.HasSuffix(entry.Name(), ".part") {
			foundPart = true
		}
	}
	if !foundPart {
		t.Fatalf("quarantine entries = %v, want canceled .part", quarantineEntries)
	}
}
