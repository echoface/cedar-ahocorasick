// Copyright 2016 Evans. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cedar

import (
	"errors"
)

var (
	// ErrNoKey not have key error
	ErrNoKey = errors.New("cedar: not have key")
	// ErrNoVal not have value error
	ErrNoVal = errors.New("cedar: not have val")
	// ErrInvalidKey invalid key error
	ErrInvalidKey = errors.New("cedar: invalid key")
	// ErrInvalidVal invalid value error
	ErrInvalidVal = errors.New("cedar: invalid val")
	// ErrInvalidData malformed serialized trie error
	ErrInvalidData = errors.New("cedar: invalid data")
)

func isReduced(reduced ...bool) bool {
	if len(reduced) > 0 && !reduced[0] {
		return false
	}

	return true
}

func (cd *Cedar) get(key []byte, from, pos int) *int32 {
	to := cd.getNode(key, from, pos)
	return &cd.array[to].BaseV
}

// getNode get the follow node by key, split by update()
func (cd *Cedar) getNode(key []byte, from, pos int) int {
	for ; pos < len(key); pos++ {
		if cd.Reduced {
			value := cd.array[from].BaseV
			if value >= 0 && value != ValLimit {
				to := cd.follow(from, 0)
				cd.array[to].BaseV = value
			}
		}

		from = cd.follow(from, key[pos])
	}

	to := from
	if cd.array[from].BaseV < 0 || !cd.Reduced {
		to = cd.follow(from, 0)
	}

	return to
}

// Jump jump a node `from` to another node by following the `path`, split by find()
func (cd *Cedar) Jump(key []byte, from int) (to int, err error) {
	// hoist the loop invariants; this is the hot path of every lookup.
	arr := cd.array
	if cd.Reduced {
		for _, k := range key {
			v := arr[from].BaseV
			if v >= 0 {
				return from, ErrNoKey
			}

			to = -(int(v) + 1) ^ int(k)
			if int(arr[to].Check) != from {
				return from, ErrNoKey
			}
			from = to
		}
		return to, nil
	}

	for _, k := range key {
		v := arr[from].BaseV
		if v < 0 {
			return from, ErrNoKey
		}

		to = int(v) ^ int(k)
		if int(arr[to].Check) != from {
			return from, ErrNoKey
		}
		from = to
	}

	return to, nil
}

// Find key from double array trie, with `from` as the cursor to traverse the nodes.
func (cd *Cedar) Find(key []byte, from int) (int, error) {
	to, err := cd.Jump(key, from)
	if cd.Reduced {
		if cd.array[to].BaseV >= 0 {
			if err == nil && to != 0 {
				return int(cd.array[to].BaseV), nil
			}
			return 0, ErrNoKey
		}
	}

	if err != nil {
		return 0, ErrNoKey
	}

	// return the value of the node if `check` is correctly marked fpr the ownership,
	// otherwise it means no value is stored.
	base := cd.array[to].base(cd.Reduced)
	if base < 0 {
		return 0, ErrNoKey
	}
	n := cd.array[base]
	if int(n.Check) != to {
		return 0, ErrNoKey
	}
	return int(n.BaseV), nil
}

// Value get the path value
func (cd *Cedar) Value(path int) (val int, err error) {
	val = int(cd.array[path].BaseV)
	if val >= 0 && cd.Reduced {
		return val, nil
	}

	to := cd.array[path].base(cd.Reduced)
	if to >= 0 && to < len(cd.array) &&
		int(cd.array[to].Check) == path && cd.array[to].BaseV >= 0 {
		return int(cd.array[to].BaseV), nil
	}

	// For non-reduced: if this IS a terminal node (0-child), BaseV is the stored value
	if !cd.Reduced && val >= 0 {
		from := int(cd.array[path].Check)
		if from >= 0 {
			base := cd.array[from].base(cd.Reduced)
			if byte(path^base) == 0 {
				return val, nil
			}
		}
	}

	return 0, ErrNoVal
}

// Insert the key for the value on []byte
func (cd *Cedar) Insert(key []byte, val int) error {
	if val < 0 || val >= ValLimit {
		return ErrInvalidVal
	}

	p := cd.get(key, 0, 0)
	*p = int32(val)

	return nil
}

// Update the key for the value, it is public interface that works on []byte
func (cd *Cedar) Update(key []byte, value int) error {
	p := cd.get(key, 0, 0)

	cur := int(*p)
	if cur == ValLimit && cd.Reduced {
		cur = 0
	}

	// values are stored as int32; reject sums that leave [0, ValLimit)
	value += cur
	if value < 0 || value >= ValLimit {
		return ErrInvalidVal
	}

	*p = int32(value)
	return nil
}

// Delete the key from the trie, the internal interface that works on []byte
func (cd *Cedar) Delete(key []byte) error {
	// move the cursor to the right place and use erase__ to delete it.
	to, err := cd.Jump(key, 0)
	if err != nil {
		return ErrNoKey
	}

	if cd.array[to].BaseV < 0 && cd.Reduced {
		base := cd.array[to].base(cd.Reduced)
		if int(cd.array[base].Check) == to {
			to = base
		}
	}

	if !cd.Reduced {
		to = cd.array[to].base(cd.Reduced)
	}

	from := to
	for to > 0 {
		from = int(cd.array[to].Check)
		base := cd.array[from].base(cd.Reduced)
		label := byte(to ^ base)

		hasSibling := cd.nInfos[to].Sibling != 0 || cd.nInfos[from].Child != label
		// if the node has siblings, then remove `e` from the sibling.
		if hasSibling {
			cd.popSibling(from, base, label)
		}

		// maintain the data structures.
		cd.pushENode(to)
		// traverse to the parent.
		to = from

		// if it has sibling then this layer has more than one nodes, then we are done.
		if hasSibling {
			break
		}
	}

	return nil
}

// Get get the key value on []byte
func (cd *Cedar) Get(key []byte) (value int, err error) {
	to, err := cd.Jump(key, 0)
	if err != nil {
		return 0, err
	}

	return cd.Value(to)
}

// ExactMatch to check if `key` is in the dictionary.
func (cd *Cedar) ExactMatch(key []byte) (int, bool) {
	from := 0
	val, err := cd.Find(key, from)
	if err != nil {
		return 0, false
	}
	return val, true
}

// PrefixMatch return the collection of the common prefix
// in the dictionary with the `key`
func (cd *Cedar) PrefixMatch(key []byte, n ...int) (ids []int) {
	num := 0
	if len(n) > 0 {
		num = n[0]
	}

	for from, i := 0, 0; i < len(key); i++ {
		to, err := cd.Jump(key[i:i+1], from)
		if err != nil {
			break
		}

		_, err = cd.Value(to)
		if err == nil {
			ids = append(ids, to)
			num--
			if num == 0 {
				return
			}
		}

		from = to
	}

	return
}

// PrefixPredict eturn the list of words in the dictionary
// that has `key` as their prefix
func (cd *Cedar) PrefixPredict(key []byte, n ...int) (ids []int) {
	num := 0
	if len(n) > 0 {
		num = n[0]
	}

	root, err := cd.Jump(key, 0)
	if err != nil {
		return
	}

	for from, err := cd.begin(root); err == nil; from, err = cd.next(from, root) {
		ids = append(ids, from)
		num--
		if num == 0 {
			return
		}
	}

	return
}

// Size returns the length of the double array; every node index is below it.
func (cd *Cedar) Size() int {
	return cd.size
}

// Children appends the labels of the edges leaving node `from` to dst, in
// sibling-chain order, skipping the terminal edge (label 0).
// Follow an edge with Jump.
func (cd *Cedar) Children(from int, dst []byte) []byte {
	base := cd.array[from].base(cd.Reduced)
	if base < 0 {
		return dst
	}

	c := cd.nInfos[from].Child
	if c == 0 {
		// the chain starts at the terminal slot: either a real 0-child, or the
		// node itself when base == from (the root); otherwise no children.
		if base != from && int(cd.array[base].Check) != from {
			return dst
		}
		c = cd.nInfos[base].Sibling
	}

	for ; c != 0; c = cd.nInfos[base^int(c)].Sibling {
		dst = append(dst, c)
	}

	return dst
}

// To get the cursor of the first leaf node starting by `from`
func (cd *Cedar) begin(from int) (to int, err error) {
	// recursively traversing down to look for the first leaf.
	for c := cd.nInfos[from].Child; c != 0; {
		from = cd.array[from].base(cd.Reduced) ^ int(c)
		c = cd.nInfos[from].Child
	}

	if cd.array[from].base(cd.Reduced) > 0 {
		return cd.array[from].base(cd.Reduced), nil
	}

	// To return the value of the leaf.
	return from, nil
}

// To move the cursor from one leaf to the next for the common prefix predict.
func (cd *Cedar) next(from int, root int) (to int, err error) {
	c := cd.nInfos[from].Sibling

	// traversing up until there is a sibling or it has reached the root.
	for c == 0 && from != root && cd.array[from].Check >= 0 {
		from = int(cd.array[from].Check)
		c = cd.nInfos[from].Sibling
	}

	if from == root || cd.array[from].Check < 0 {
		return 0, ErrNoKey
	}

	// it has a sibling so we leverage on `begin` to traverse the subtree down again.
	from = cd.array[cd.array[from].Check].base(cd.Reduced) ^ int(c)
	return cd.begin(from)
}

// Array returns the underlying double-array nodes. The result aliases the
// trie's storage and must not be modified.
func (cd *Cedar) Array() []Node { return cd.array }

// NInfos returns sibling/child metadata. The result aliases the trie's
// storage and must not be modified.
func (cd *Cedar) NInfos() []NInfo { return cd.nInfos }

// Blocks returns per-256-slot allocation metadata. The result aliases the
// trie's storage and must not be modified.
func (cd *Cedar) Blocks() []Block { return cd.blocks }

// NodeBase decodes the base offset of a node for the trie's mode.
func (cd *Cedar) NodeBase(i int) int { return cd.array[i].base(cd.Reduced) }
