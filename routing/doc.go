// Package routing models explicit track-patch, output-bus and send graphs.
//
// Node and port IDs are the only routing identities. Workspace validation
// reports missing endpoints and directed cycles. Batch operations are applied
// to a clone and committed only when every operation remains valid, so a
// missing endpoint can never leave a partially migrated graph behind.
package routing
