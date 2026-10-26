# cedar

[![Go Reference](https://pkg.go.dev/badge/github.com/vcaesar/cedar.svg)](https://pkg.go.dev/github.com/vcaesar/cedar)
[![Go Report Card](https://goreportcard.com/badge/github.com/vcaesar/cedar)](https://goreportcard.com/report/github.com/vcaesar/cedar)
[![Build Status](https://github.com/vcaesar/cedar/actions/workflows/go.yml/badge.svg)](https://github.com/vcaesar/cedar/actions/workflows/go.yml)

Updatable double-array tries and Aho-Corasick matchers for Go, with a
fixed-layout snapshot format that can be restored through `mmap` without
rebuilding the automaton.

## Packages

| Package | API | Purpose |
| --- | --- | --- |
| `cedar` | `New`, `Insert`, `Get`, `Jump`, `PrefixMatch`, `SaveSnapshot`, `OpenSnapshot` | Updatable double-array trie |
| `aho` | `New`, `Insert`, `Delete`, `Match`, `Compile`, `SaveSnapshot`, `OpenSnapshot` | Aho-Corasick matcher built on `cedar` |

The root package has no aho types: the trie layer and matcher layer are
independent public APIs, and `aho` depends on `cedar`, never the reverse.

## Install

```sh
go get github.com/vcaesar/cedar
```

## Trie

```go
package main

import (
	"fmt"

	"github.com/vcaesar/cedar"
)

func main() {
	trie := cedar.New()

	_ = trie.Insert([]byte("she"), 1)
	_ = trie.Insert([]byte("hers"), 2)

	if v, ok := trie.ExactMatch([]byte("she")); ok {
		fmt.Println("exact:", v)
	}

	node, err := trie.Jump([]byte("she"), 0)
	if err == nil {
		v, _ := trie.Value(node)
		fmt.Println("jump:", node, v)
	}

	// Every dictionary key that is a prefix of the query.
	for _, id := range trie.PrefixMatch([]byte("hers"), 0) {
		fmt.Println("prefix:", id)
	}
}
```

Keys are byte strings. Values are non-negative `int32` values (`0` through
`math.MaxInt32-1`). The trie supports streaming inserts and deletes at any
time; it is not safe for concurrent mutation.

## Aho-Corasick

```go
package main

import (
	"fmt"

	"github.com/vcaesar/cedar/aho"
)

func main() {
	m := aho.NewStrings("he", "she", "his", "hers")
	_ = m.Insert([]byte("太阳系"), 100)

	text := []byte("ushers 太阳系")
	for _, t := range m.Match(text) {
		fmt.Printf("value=%d at=%d len=%d key=%q\n",
			t.Value, t.At, t.Len, m.Key(text, t))
	}
}
```

Output:

```text
value=1 at=1 len=3 key="she"
value=0 at=2 len=2 key="he"
value=3 at=2 len=4 key="hers"
value=100 at=7 len=9 key="太阳系"
```

Failure, output, and depth links are built lazily by `Match`, or explicitly
with `Compile`. Any `Insert` or `Delete` invalidates them and the next match
rebuilds the links.

Patterns may contain any byte except NUL, which cedar reserves as its key
terminator. NUL in input text resets the automaton because it cannot be part
of a match.

## Snapshots

Snapshots are fixed-layout, CRC-checked files. Saves are atomic: the writer
creates a temporary file, fsyncs it, then renames it over the destination.
A failed save leaves any previous snapshot intact.

```go
// Matcher snapshot: includes trie plus failure/output/depth links.
m := aho.NewStrings("he", "she", "hers")
m.Compile()

if err := m.SaveSnapshot("matcher.snp"); err != nil {
	panic(err)
}

s, err := aho.OpenSnapshot("matcher.snp")
if err != nil {
	panic(err)
}
defer s.Close()

for _, t := range s.Matcher().MatchString("ushers") {
	fmt.Println(t.Value, t.At, t.Len)
}
```

```go
// Trie-only snapshot.
trie := cedar.New()
_ = trie.Insert([]byte("key"), 1)

if err := trie.SaveSnapshot("trie.snp"); err != nil {
	panic(err)
}

s, err := cedar.OpenSnapshot("trie.snp")
if err != nil {
	panic(err)
}
defer s.Close()

v, _ := s.Cedar().Get([]byte("key"))
fmt.Println(v)
```

On Unix systems `OpenSnapshot` maps the file read-only and exposes slices
directly into the mapping. Loaded matchers are already compiled: opening a
snapshot does not rebuild failure links. Other platforms use a portable
read-into-memory fallback.

Snapshot properties:

- v1 format is immutable; mutate an in-memory trie and save a new file
- layout is little-endian only
- both header and payload bytes are protected by CRC-32
- cedar-only files and matcher files are distinguished and cannot be confused

See [SNAPSHOT.md](SNAPSHOT.md) for byte layout, section kinds, validation
rules, and the save protocol.

## Portable persistence

For smaller dictionaries, the original gob and JSON codecs remain available:

```go
m.SaveToFile("patterns.gob", "gob")

loaded := aho.New()
loaded.LoadFromFile("patterns.gob", "gob")
```

These formats copy and decode every element and rebuild AC links on the next
match. Prefer snapshots for large dictionaries or fast startup.

## Development

```sh
go test ./...
go test -race ./...
go test -bench . -run '^$' ./...
```

`aho/snapshot_test.go` covers snapshot round-trips and corrupted-file
rejection. `aho/badcase_test.go` and `aho/fuzz_test.go` retain regression,
determinism, and fuzz coverage for the matcher.

## License

BSD-2-Clause. See [LICENSE](LICENSE).

## References

- Naoki Yoshinaga, [cedar: C++ efficiently-updatable double-array trie](http://www.tkl.iis.u-tokyo.ac.jp/~ynaga/cedar/)
- Yoshinaga and Kitsuregawa, [A Self-adaptive Classifier for Efficient Text-stream Processing](http://www.tkl.iis.u-tokyo.ac.jp/~ynaga/papers/ynaga-coling2014.pdf), COLING 2014
