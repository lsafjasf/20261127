package checkpoint

import (
	"context"
	"fmt"
	"io"
)

// RunContext gives a task access to validated dependency artifacts and the
// durable store.
type RunContext struct {
	store        *Store
	taskID       string
	dependencies map[string]*Artifact
}

// TaskID returns the current graph node id.
func (r *RunContext) TaskID() string { return r.taskID }

// Store exposes the underlying checkpoint store.
func (r *RunContext) Store() *Store { return r.store }

// Dependency returns the committed artifact for a direct dependency.
func (r *RunContext) Dependency(id string) (*Artifact, error) {
	artifact := r.dependencies[id]
	if artifact == nil {
		return nil, fmt.Errorf("%w: %s", ErrDependencyMissing, id)
	}
	return artifact, nil
}

// OpenDependency opens and checksum-verifies a dependency artifact.
func (r *RunContext) OpenDependency(id string) (io.ReadCloser, *Artifact, error) {
	artifact, err := r.Dependency(id)
	if err != nil {
		return nil, nil, err
	}
	reader, err := r.store.OpenArtifact(artifact)
	if err != nil {
		return nil, nil, err
	}
	return reader, artifact, nil
}

// PutBytes commits a reusable intermediate artifact.
func (r *RunContext) PutBytes(ctx context.Context, data []byte, mediaType string) (*Artifact, error) {
	return r.store.PutBytes(ctx, data, mediaType)
}

// PutReader commits a reusable intermediate artifact from a stream.
func (r *RunContext) PutReader(ctx context.Context, reader io.Reader, mediaType string) (*Artifact, error) {
	return r.store.PutReader(ctx, reader, mediaType)
}

// Export publishes a final output file atomically.
func (r *RunContext) Export(ctx context.Context, relativePath string, artifact *Artifact) (string, error) {
	return r.store.ExportArtifact(ctx, artifact, relativePath)
}

// ExportReader publishes streamed final output atomically.
func (r *RunContext) ExportReader(ctx context.Context, relativePath string, reader io.Reader) (string, error) {
	return r.store.ExportReader(ctx, relativePath, reader)
}
