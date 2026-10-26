package cedar

import (
	"os"
	"path/filepath"
	"testing"
)

func buildSnapshotTrie(t *testing.T) *Cedar {
	t.Helper()
	cd := New()
	for i, k := range []string{"he", "she", "her", "hers", "here", "hereby"} {
		if err := cd.Insert([]byte(k), i); err != nil {
			t.Fatal(err)
		}
	}
	return cd
}

func TestSnapshotRoundTrip(t *testing.T) {
	cd := buildSnapshotTrie(t)
	path := filepath.Join(t.TempDir(), "cedar.snp")
	if err := cd.SaveSnapshot(path); err != nil {
		t.Fatal(err)
	}

	s, err := OpenSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.Flags()&FlagAhoLinks != 0 {
		t.Fatal("cedar snapshot should not contain AC links")
	}
	got := s.Cedar()
	for i, k := range []string{"he", "she", "her", "hers", "here", "hereby"} {
		v, err := got.Get([]byte(k))
		if err != nil || v != i {
			t.Fatalf("get %q: %v %d want %d", k, err, v, i)
		}
	}
	if _, err := got.Get([]byte("missing")); err == nil {
		t.Fatal("missing key unexpectedly found")
	}
}

func TestSnapshotAtomicSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cedar.snp")
	if err := buildSnapshotTrie(t).SaveSnapshot(path); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Save a different trie over the same path; a successful save must replace it wholly.
	cd := New()
	if err := cd.Insert([]byte("different"), 1); err != nil {
		t.Fatal(err)
	}
	if err := cd.SaveSnapshot(path); err != nil {
		t.Fatal(err)
	}
	s, err := OpenSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Cedar().Get([]byte("he")); err == nil {
		t.Fatal("old key survived replacement")
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "cedar.snp" {
			t.Fatalf("temporary file leaked: %s", e.Name())
		}
	}
	_ = before
}

func corruptSnapshot(t *testing.T, mutate func(b []byte)) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cedar.snp")
	if err := buildSnapshotTrie(t).SaveSnapshot(path); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	mutate(b)
	return b
}

func TestSnapshotRejectsCorruption(t *testing.T) {
	tests := []struct {
		name string
		mut  func(b []byte)
	}{
		{"magic", func(b []byte) { b[0] ^= 0xff }},
		{"version", func(b []byte) { b[8] = 99 }},
		{"data", func(b []byte) { b[len(b)/2] ^= 0xff }},
		{"footer", func(b []byte) { b[len(b)-64+16] ^= 0xff }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := corruptSnapshot(t, tc.mut)
			path := filepath.Join(t.TempDir(), "bad.snp")
			if err := os.WriteFile(path, b, 0666); err != nil {
				t.Fatal(err)
			}
			if _, err := OpenSnapshot(path); err == nil {
				t.Fatal("corrupt snapshot accepted")
			}
		})
	}
}

func TestSnapshotCloseTwice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cedar.snp")
	if err := buildSnapshotTrie(t).SaveSnapshot(path); err != nil {
		t.Fatal(err)
	}
	s, err := OpenSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}
