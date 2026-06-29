// Copyright 2016 Evans. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cedar

// AhoCorasick implements the Aho-Corasick string matching algorithm
// on top of a double-array trie.
type AhoCorasick struct {
	*Cedar
	fail    []int   // failure links for each node
	outputs [][]int // pattern values that match at each node (including via suffix chain)
	built   bool
}

// MatchResult represents a single match found by the AC automaton.
type MatchResult struct {
	Value    int    // the value (pattern index) of the matched pattern
	Position int    // the end position (byte offset) in the text, 1-based
}

// NewAhoCorasick creates a new Aho-Corasick automaton.
func NewAhoCorasick(reduced ...bool) *AhoCorasick {
	return &AhoCorasick{
		Cedar: New(reduced...),
	}
}

// Build constructs failure links for the Aho-Corasick automaton.
// Must be called after all patterns have been inserted and before Match.
func (ac *AhoCorasick) Build() {
	size := cap(ac.array)
	ac.fail = make([]int, size)
	ac.outputs = make([][]int, size)

	queue := make([]int, 0)

	// Initialize root's children: set fail to root (0)
	ac.initChildren(0, &queue)

	// BFS to build failure links
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]

		base := ac.array[node].base(ac.Reduced)
		if base < 0 {
			continue
		}

		// Iterate all children (label != 0, non-terminal edges)
		firstLabel := ac.nInfos[node].child
		if firstLabel == 0 {
			// Terminal edge first; siblings are non-terminal children
			termNode := base // base ^ 0
			sibLabel := ac.nInfos[termNode].sibling
			ac.processChildren(node, base, sibLabel, &queue)
		} else {
			ac.processChildren(node, base, firstLabel, &queue)
		}
	}

	ac.built = true
}

// initChildren initializes the failure links for the root's direct children.
func (ac *AhoCorasick) initChildren(root int, queue *[]int) {
	base := ac.array[root].base(ac.Reduced)
	if base < 0 {
		return
	}

	firstLabel := ac.nInfos[root].child
	if firstLabel == 0 {
		// Terminal at root: check value
		if base < len(ac.array) && ac.array[base].check == root {
			if val, err := ac.Value(base); err == nil {
				ac.outputs[base] = append(ac.outputs[base], val)
				ac.fail[base] = 0
			}
		}
		sibLabel := ac.nInfos[base].sibling
		ac.collectChildren(root, base, sibLabel, queue)
	} else {
		ac.collectChildren(root, base, firstLabel, queue)
	}
}

// collectChildren iterates non-terminal children of root, setting fail=0.
func (ac *AhoCorasick) collectChildren(root, base int, label byte, queue *[]int) {
	for label != 0 {
		childNode := base ^ int(label)
		if childNode < len(ac.array) && ac.array[childNode].check == root {
			ac.fail[childNode] = 0
			if val, err := ac.Value(childNode); err == nil {
				ac.outputs[childNode] = append(ac.outputs[childNode], val)
			}
			*queue = append(*queue, childNode)
		}
		label = ac.nInfos[childNode].sibling
	}
}

// processChildren processes non-terminal children of a node during BFS.
func (ac *AhoCorasick) processChildren(node, base int, label byte, queue *[]int) {
	for label != 0 {
		childNode := base ^ int(label)
		if childNode >= len(ac.array) || ac.array[childNode].check != node {
			label = ac.nInfos[childNode].sibling
			continue
		}

		// Find failure link for this child
		fail := ac.fail[node]
		ac.fail[childNode] = ac.findFailLink(fail, label)

		// Collect output values
		if val, err := ac.Value(childNode); err == nil {
			ac.outputs[childNode] = append(ac.outputs[childNode], val)
		}
		// Inherit outputs from failure node
		failNode := ac.fail[childNode]
		if failNode >= 0 && failNode < len(ac.outputs) {
			ac.outputs[childNode] = append(ac.outputs[childNode], ac.outputs[failNode]...)
		}

		*queue = append(*queue, childNode)
		label = ac.nInfos[childNode].sibling
	}
}

// findFailLink follows failure links to find the goto(node, label) target.
func (ac *AhoCorasick) findFailLink(node int, label byte) int {
	for node > 0 {
		base := ac.array[node].base(ac.Reduced)
		if base >= 0 {
			nextNode := base ^ int(label)
			if nextNode < len(ac.array) && ac.array[nextNode].check == node {
				return nextNode
			}
		}
		node = ac.fail[node]
	}

	// Try root
	base := ac.array[0].base(ac.Reduced)
	if base >= 0 {
		nextNode := base ^ int(label)
		if nextNode < len(ac.array) && ac.array[nextNode].check == 0 {
			return nextNode
		}
	}

	return 0
}

// gotoFunc follows the goto transition from node with the given byte label.
// Returns the next node, or 0 (root) if no transition exists.
func (ac *AhoCorasick) gotoFunc(node int, label byte) int {
	base := ac.array[node].base(ac.Reduced)
	if base < 0 {
		return 0
	}

	nextNode := base ^ int(label)
	if nextNode < len(ac.array) && ac.array[nextNode].check == node {
		return nextNode
	}

	return 0
}

// Match finds all occurrences of the patterns in the given text.
// Returns a list of MatchResult, each containing the pattern value and end position.
// Results are in the order they are found (by position, then by pattern value).
func (ac *AhoCorasick) Match(text []byte) []MatchResult {
	if !ac.built {
		ac.Build()
	}

	results := make([]MatchResult, 0)
	node := 0

	for pos := 0; pos < len(text); pos++ {
		ch := text[pos]

		// Follow failure links until we find a goto transition or reach root
		for node > 0 && !ac.hasGoto(node, ch) {
			node = ac.fail[node]
		}

		// Follow goto transition (may be from root)
		nextNode := ac.gotoFunc(node, ch)
		if nextNode > 0 {
			node = nextNode
		}

		// Collect all outputs at this node
		if node < len(ac.outputs) && len(ac.outputs[node]) > 0 {
			for _, val := range ac.outputs[node] {
				results = append(results, MatchResult{
					Value:    val,
					Position: pos + 1,
				})
			}
		}
	}

	return results
}

// hasGoto checks if there is a valid goto transition from node with the given label.
func (ac *AhoCorasick) hasGoto(node int, label byte) bool {
	return ac.gotoFunc(node, label) > 0
}

// MatchAll finds all unique pattern values that appear in the text.
func (ac *AhoCorasick) MatchAll(text []byte) []int {
	results := ac.Match(text)
	seen := make(map[int]struct{})
	unique := make([]int, 0)

	for _, r := range results {
		if _, ok := seen[r.Value]; !ok {
			seen[r.Value] = struct{}{}
			unique = append(unique, r.Value)
		}
	}

	return unique
}