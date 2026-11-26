# dcpcheck

`dcpcheck` is a read-only DCP/CPL timing and reference checker implemented with
the Go standard library only. It parses the CPL, optionally resolves every asset
Id through `PKL_*.xml` and `ASSETMAP`, and reports the first timeline moment that
the CPL requests beyond an asset's `IntrinsicDuration`.

## Usage

```sh
go run . -root /path/to/dcp
go run . -cpl /path/to/CPL_example.xml -json
go test ./...
```

The tool only opens XML and chunk files for reading; it never writes, repairs, or
renames package contents. Exit status is `0` when all CPL source ranges are
available, `1` when a timing or reference problem is found, and `2` when the
input cannot be loaded or parsed.

## First unplayable moment

For an asset with a valid `EntryPoint`, `IntrinsicDuration`, `Duration`, and
asset `EditRate`:

```text
available_units       = IntrinsicDuration - EntryPoint
first_missing_output  = available_units
first_missing_source  = IntrinsicDuration
first_unplayable_time = reel_start + first_missing_output / EditRate
CPL_frame             = first_unplayable_time * Composition_EditRate
```

If `EntryPoint > IntrinsicDuration`, the first requested output unit is already
unavailable. The report gives exact decimal-second and rational frame values;
SMPTE timecode is added for integral frame rates.

Asset references are normalized as lowercase UUIDs and checked against PKL and
AssetMap Ids. AssetMap chunk paths are resolved and stat-checked; AssetMap
`Chunk/Length` and the sum of chunk sizes against PKL `Size` are verified when
those fields are present. The checker does not demux MXF essence or verify hashes.

## Fixture

`testdata/short-asset` contains three referenced assets. The subtitle asset has
`EntryPoint=5`, `IntrinsicDuration=124`, and `Duration=120` at `24/1`. It has 119
available output units, so the first missing timeline frame is CPL frame `119`,
at `4.958333333 s` / `00:00:04:23`; its first missing source frame is `124`.
