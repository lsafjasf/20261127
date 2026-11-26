#!/usr/bin/env bash
# End-to-end demonstration:
#   1. reproduce non-deterministic packaging (legacy packer),
#   2. classify every difference axis,
#   3. prove the canonical packer is reproducible (tar + gzip),
#   4. prove a real content change is detected as a media difference.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'echo "(scratch left in: $WORK)"' EXIT

go -C "$ROOT" build -o "$WORK/mediabundle" ./cmd/mediabundle
MB="$WORK/mediabundle"

mkdir -p "$WORK/src/clips"
printf 'alpha payload' > "$WORK/src/clips/a.txt"
printf 'beta payload'  > "$WORK/src/b.txt"
head -c 256 /dev/urandom > "$WORK/src/noise.bin"

echo '== 1. legacy pack twice: hashes SHOULD differ =='
"$MB" legacy-pack --src "$WORK/src" --out "$WORK/legacy1.tar.gz" --gzip
sleep 1.1
"$MB" legacy-pack --src "$WORK/src" --out "$WORK/legacy2.tar.gz" --gzip

echo
echo '== 2. diff explains: timestamps + order + random id, but equivalent media =='
"$MB" diff --a "$WORK/legacy1.tar.gz" --b "$WORK/legacy2.tar.gz"

echo
echo '== 3. canonical pack twice: hashes MUST be equal (tar and gzip) =='
"$MB" pack --src "$WORK/src" --out "$WORK/can1.tar"
"$MB" pack --src "$WORK/src" --out "$WORK/can2.tar"
"$MB" pack --src "$WORK/src" --out "$WORK/can1.tar.gz" --gzip
"$MB" pack --src "$WORK/src" --out "$WORK/can2.tar.gz" --gzip

echo
echo '== 4. verify subcommand =='
"$MB" verify --src "$WORK/src" --gzip

echo
echo '== 5. real media change is a media-different verdict, not cosmetic =='
cp -a "$WORK/src" "$WORK/src2"
printf 'MUTATED' >> "$WORK/src2/b.txt"
"$MB" pack --src "$WORK/src"  --out "$WORK/orig.tar"
"$MB" pack --src "$WORK/src2" --out "$WORK/mutated.tar"
"$MB" diff --a "$WORK/orig.tar" --b "$WORK/mutated.tar"
