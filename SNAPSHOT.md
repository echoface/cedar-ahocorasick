# Cedar Snapshot Format

Status: Draft / Implementation v1

## Goals

- Zero-copy read-only restore through `mmap`.
- One atomic file replace when saving.
- Persist Aho-Corasick failure/output/depth links to avoid recompiling after load.
- Keep cedar's updatable trie metadata so a snapshot can also be resumed for inserts/deletes.
- Simple enough to validate in a few hundred lines.

## Non-goals

- Cross-endian portability. v1 is little-endian only and rejects other byte orders.
- In-place writable mmap. Saving always writes a new file and atomically renames it.
- Compressed format. The goal is predictable offsets, not minimal disk usage.

## Layers

- Package `cedar` owns the raw trie layout and trie-only snapshot API.
- Package `aho` extends that layout with AC links and exposes matcher-level save/load APIs.

## File layout

All integers are little-endian. Offsets are from the beginning of the file.
Every section starts at an 8-byte boundary.

```text
+-------------+ aligned 0
| FileHeader  | 64 bytes
+-------------+
| Section[]   | N * 32 bytes, N fixed by version
+-------------+
| CedarHeader | 64 bytes
+-------------+
| Array       | Size * 8  (Node: BaseV int32, Check int32)
+-------------+
| NInfo       | Size * 2  (Sibling byte, Child byte)
+-------------+
| Block       | Size/256 * 24
+-------------+ only in aho snapshots
| Fails       | Size * 4  (int32 node index)
+-------------+
| Outputs     | Size * 4  (int32 node index, -1 = none)
+-------------+
| Depth       | Size * 4  (int32 byte length)
+-------------+
| Footer      | 64 bytes: magic, header CRC32, data CRC32
+-------------+
```

## Headers

```go
type fileHeader struct {
    Magic          uint64   // "CEDARSNP" (little-endian constant)
    Version        uint64   // 1
    ByteOrder      uint64   // 0x0102030405060708 on little-endian hosts
    Flags          uint64   // bit 0: reduced trie, bit 1: has AC links
    Sections       uint64   // number of section records
    Reserved       [3]uint64
}

type section struct {
    Kind           uint64   // see section kinds
    Offset         uint64
    Size           uint64   // section bytes, not element count
    Elements       uint64   // element count where applicable, otherwise 0
    Reserved       uint64
}

type cedarHeader struct {
    Size           uint64   // number of Node/NInfo slots; multiple of 256
    Capacity       uint64   // always == Size for v1 snapshots
    NumBlocks      uint64   // Size / 256
    Ordered        uint64   // 0/1
    MaxTrial       uint64
    HeadFull       int64
    HeadClosed     int64
    HeadOpen       int64
}
```

### Section kinds

```go
const (
    SectionCedarHeader uint64 = 1
    SectionArray       uint64 = 2
    SectionNInfo       uint64 = 3
    SectionBlock       uint64 = 4
    SectionFails       uint64 = 5
    SectionOutputs     uint64 = 6
    SectionDepth       uint64 = 7
)
```

### AC link invariants

- All three AC arrays have exactly `Size` elements.
- `0 <= Fails[i] < Size`.
- `Outputs[i] == -1 || 0 <= Outputs[i] < Size`.
- `0 <= Depth[i] <= MaxInt32`.
- Root fail/root output are zero/-1 respectively.
- Output links form a chain terminating at -1; cycles are rejected.

## Validation levels

1. Structural: magic/version/endian/section bounds/alignment/lengths.
2. CRC: header CRC and data CRC are checked before exposing mmap-backed slices.
3. Semantic: snapshot loading validates block and AC invariants listed above.

## Save protocol

1. Encode headers into memory and compute section offsets.
2. Write to `<path>.tmp-<pid>` with section bytes.
3. `fsync` the file.
4. Atomically `rename` over `<path>`.
5. Optionally fsync the parent directory.

A failed save never changes an existing snapshot.

## mmap API

- `cedar.OpenSnapshot(path string) (*Snapshot, error)`
- `(*Snapshot).Cedar() *Cedar`
- `(*Snapshot).Close() error`

- `aho.OpenSnapshot(path string) (*Snapshot, error)`
- `(*Snapshot).Matcher() *Matcher`
- `(*Snapshot).Close() error`

Snapshots are immutable. Mutating APIs reset the loaded object to mutable heap
storage first; the mmap itself is never written through.
