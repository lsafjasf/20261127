#!/usr/bin/env bash
# Regenerate the deterministic reference case, rebuild the hash index and
# report, then gate the result with `verify`. Human review files under
# case/review/ are intentionally never overwritten by init-case.
set -euo pipefail
cd "$(dirname "$0")/.."

export GOPROXY=off
export GOSUMDB=off
export GOTOOLCHAIN=local

go run ./cmd/evidencechain init-case --dir case
go run ./cmd/evidencechain index --case case --chunk-size 128
go run ./cmd/evidencechain report --case case --out FORENSIC-REPORT.md
# Final independent gate; exits 3 if the chain or any human judgment fails audit.
go run ./cmd/evidencechain verify --case case --write-json case/out/verification.json
