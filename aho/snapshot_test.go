package aho

import (
	"path/filepath"
	"testing"

	"github.com/vcaesar/cedar"
)

func snapshotMatcher(t *testing.T) *Matcher {
	t.Helper()
	m := NewStrings("he", "she", "his", "hers", "太阳系")
	if err := m.Insert([]byte("hereby"), 9); err != nil {
		t.Fatal(err)
	}
	m.Compile()
	return m
}

func TestSnapshotRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "matcher.snp")
	m := snapshotMatcher(t)
	want := m.MatchString("ushers 太阳系 hereby")
	if err := m.SaveSnapshot(path); err != nil {
		t.Fatal(err)
	}

	s, err := OpenSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got := s.Matcher().MatchString("ushers 太阳系 hereby")
	if len(got) != len(want) {
		t.Fatalf("match count %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("match[%d] = %+v want %+v", i, got[i], want[i])
		}
	}
}

func TestSnapshotNoRecompile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "matcher.snp")
	m := snapshotMatcher(t)
	m.Compile()
	if err := m.SaveSnapshot(path); err != nil {
		t.Fatal(err)
	}
	s, err := OpenSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !s.Matcher().compiled {
		t.Fatal("loaded matcher should be compiled")
	}
	if len(s.Matcher().fails) == 0 || len(s.Matcher().outputs) == 0 || len(s.Matcher().depth) == 0 {
		t.Fatal("AC links not restored")
	}
}

func TestSnapshotRejectsCedarOnly(t *testing.T) {
	dir := t.TempDir()
	cdPath := filepath.Join(dir, "cedar.snp")
	if err := snapshotMatcher(t).Cedar().SaveSnapshot(cdPath); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenSnapshot(cdPath); err == nil {
		t.Fatal("cedar-only snapshot accepted as matcher")
	}

	mPath := filepath.Join(dir, "m.snp")
	if err := snapshotMatcher(t).SaveSnapshot(mPath); err != nil {
		t.Fatal(err)
	}
	if _, err := cedar.OpenSnapshot(mPath); err == nil {
		t.Fatal("matcher snapshot accepted as cedar-only")
	}
}
