// Copyright 2016 Evans. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package aho

import (
	"fmt"
	"os"

	"github.com/vcaesar/cedar"
)

// SaveSnapshot writes the compiled matcher as a fixed-layout snapshot and
// atomically replaces path. The failure/output links are included, so loading
// does not recompile the automaton.
func (m *Matcher) SaveSnapshot(path string) error {
	if !m.compiled {
		m.Compile()
	}
	if len(m.fails) != m.da.Size() || len(m.outputs) != m.da.Size() || len(m.depth) != m.da.Size() {
		return fmt.Errorf("aho: stale links: got %d/%d/%d want %d",
			len(m.fails), len(m.outputs), len(m.depth), m.da.Size())
	}
	return m.da.SaveSnapshotFile(path, m.fails, m.outputs, m.depth)
}

// Snapshot is an immutable mmap-backed Aho-Corasick matcher.
type Snapshot struct {
	cedar *cedar.Snapshot
	m     *Matcher
}

// Matcher returns the matcher backed by the mmap mapping.
func (s *Snapshot) Matcher() *Matcher { return s.m }

// Close releases the underlying mapping. It is safe to call twice.
func (s *Snapshot) Close() error { return s.cedar.Close() }

// OpenSnapshot opens a matcher snapshot written by Matcher.SaveSnapshot.
func OpenSnapshot(path string) (*Snapshot, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cs, err := cedar.OpenSnapshotFile(path, f, cedar.FlagAhoLinks)
	if err != nil {
		return nil, err
	}
	links := cs.AhoLinks()
	m := &Matcher{
		da:       cs.Cedar(),
		fails:    links.Fails,
		outputs:  links.Outputs,
		depth:    links.Depth,
		compiled: true,
	}
	return &Snapshot{cedar: cs, m: m}, nil
}
