# 20261127

Repository initialized for 20261127.

## Insert / overwrite ripple check

Go reference model and deterministic 20-step random audit live in
[`RIPPLE.md`](RIPPLE.md).

```bash
go test ./...
go run ./cmd/ripplecheck -seed 20261127 -n 20
```

The command writes the full clip, transition, fade, keyframe, envelope,
duration, and marker trace to `artifacts/ripple-audit.json`.
