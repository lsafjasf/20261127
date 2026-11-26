// Command evidencechain is the root-package entry point; the full CLI also
// lives at ./cmd/evidencechain. Both call the same offline implementation.
package main

import "evidencechain/internal/cli"

func main() { cli.Run() }
