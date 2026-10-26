// Copyright 2016 Evans. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cedar

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"hash/crc32"
	"os"
	"path/filepath"
	"unsafe"
)

// Snapshot format constants. See SNAPSHOT.md for the full layout.
const (
	SnapshotVersion = 1

	FlagReduced  uint64 = 1 << 0
	FlagAhoLinks uint64 = 1 << 1

	SectionCedarHeader uint64 = 1
	SectionArray       uint64 = 2
	SectionNInfo       uint64 = 3
	SectionBlock       uint64 = 4
	SectionFails       uint64 = 5
	SectionOutputs     uint64 = 6
	SectionDepth       uint64 = 7

	fileHeaderSize  = 64
	sectionSize     = 32
	cedarHeaderSize = 64
	footerSize      = 64
)

var (
	snapshotMagic = binary.LittleEndian.Uint64([]byte("CEDARSNP\x00"))

	ErrInvalidSnapshot = errors.New("cedar: invalid snapshot")
	ErrSnapshotVersion = errors.New("cedar: unsupported snapshot version")
)

// snapshotFileHeader is the fixed-size prefix shared by cedar and aho snapshots.
type snapshotFileHeader struct {
	Magic     uint64
	Version   uint64
	ByteOrder uint64
	Flags     uint64
	Sections  uint64
	Reserved  [3]uint64
}

type snapshotSection struct {
	Kind     uint64
	Offset   uint64
	Size     uint64
	Elements uint64
	Reserved uint64
}

type snapshotCedarHeader struct {
	Size       uint64
	Capacity   uint64
	NumBlocks  uint64
	Ordered    uint64
	MaxTrial   uint64
	HeadFull   int64
	HeadClosed int64
	HeadOpen   int64
}

type snapshotFooter struct {
	Magic     uint64
	HeaderCRC uint64
	DataCRC   uint64
	Reserved  [5]uint64
}

func getFooter(b []byte) snapshotFooter {
	return snapshotFooter{
		Magic:     binary.LittleEndian.Uint64(b),
		HeaderCRC: binary.LittleEndian.Uint64(b[8:]),
		DataCRC:   binary.LittleEndian.Uint64(b[16:]),
	}
}

// putFileHeader encodes h into exactly 64 bytes.
func putFileHeader(dst []byte, h snapshotFileHeader) {
	off := 0
	putU64 := func(v uint64) {
		binary.LittleEndian.PutUint64(dst[off:], v)
		off += 8
	}
	putU64(h.Magic)
	putU64(h.Version)
	putU64(h.ByteOrder)
	putU64(h.Flags)
	putU64(h.Sections)
	putU64(h.Reserved[0])
	putU64(h.Reserved[1])
	putU64(h.Reserved[2])
}

func getFileHeader(b []byte) snapshotFileHeader {
	g := func(i int) uint64 { return binary.LittleEndian.Uint64(b[i*8:]) }
	return snapshotFileHeader{
		Magic: g(0), Version: g(1), ByteOrder: g(2), Flags: g(3),
		Sections: g(4), Reserved: [3]uint64{g(5), g(6), g(7)},
	}
}

func putSection(dst []byte, s snapshotSection) {
	off := 0
	putU64 := func(v uint64) {
		binary.LittleEndian.PutUint64(dst[off:], v)
		off += 8
	}
	putU64(s.Kind)
	putU64(s.Offset)
	putU64(s.Size)
	putU64(s.Elements)
	putU64(s.Reserved)
}

func getSection(b []byte) snapshotSection {
	g := func(i int) uint64 { return binary.LittleEndian.Uint64(b[i*8:]) }
	return snapshotSection{Kind: g(0), Offset: g(1), Size: g(2), Elements: g(3), Reserved: g(4)}
}

func putCedarHeader(dst []byte, h snapshotCedarHeader) {
	off := 0
	putU64 := func(v uint64) { binary.LittleEndian.PutUint64(dst[off:], v); off += 8 }
	putI64 := func(v int64) { binary.LittleEndian.PutUint64(dst[off:], uint64(v)); off += 8 }
	putU64(h.Size)
	putU64(h.Capacity)
	putU64(h.NumBlocks)
	putU64(h.Ordered)
	putU64(h.MaxTrial)
	putI64(h.HeadFull)
	putI64(h.HeadClosed)
	putI64(h.HeadOpen)
}

func getCedarHeader(b []byte) snapshotCedarHeader {
	gu := func(i int) uint64 { return binary.LittleEndian.Uint64(b[i*8:]) }
	gi := func(i int) int64 { return int64(gu(i)) }
	return snapshotCedarHeader{
		Size: gu(0), Capacity: gu(1), NumBlocks: gu(2), Ordered: gu(3),
		MaxTrial: gu(4), HeadFull: gi(5), HeadClosed: gi(6), HeadOpen: gi(7),
	}
}

func putFooter(dst []byte, f snapshotFooter) {
	off := 0
	putU64 := func(v uint64) { binary.LittleEndian.PutUint64(dst[off:], v); off += 8 }
	putU64(f.Magic)
	putU64(f.HeaderCRC)
	putU64(f.DataCRC)
	for _, v := range f.Reserved {
		putU64(v)
	}
}

func align8(n int) int { return (n + 7) &^ 7 }

// Snapshot is an immutable, mmap-backed cedar trie. Close releases the mapping.
type Snapshot struct {
	data     []byte
	unmap    func() error
	cd       *Cedar
	sections map[uint64]snapshotSection
	flags    uint64
}

type sectionData struct {
	array   []Node
	nInfos  []NInfo
	blocks  []Block
	fails   []int32
	outputs []int32
	depth   []int32
}

// Cedar returns the snapshot's trie. Mutating it copies storage first.
func (s *Snapshot) Cedar() *Cedar { return s.cd }

// Flags returns format flags such as FlagReduced and FlagAhoLinks.
func (s *Snapshot) Flags() uint64 { return s.flags }

// Close unmaps the snapshot. Calling Close twice is safe.
func (s *Snapshot) Close() error {
	if s == nil || s.unmap == nil {
		return nil
	}
	err := s.unmap()
	s.unmap = nil
	s.data = nil
	return err
}

// SaveSnapshot writes cd as a version-1 snapshot and atomically replaces path.
func (cd *Cedar) SaveSnapshot(path string) error {
	w, err := newSnapshotWriter(cd.Size(), 0)
	if err != nil {
		return err
	}
	// v1 snapshots are compact and immutable; capacity equals size.
	h := snapshotCedarHeader{
		Size: uint64(cd.size), Capacity: uint64(cd.size), NumBlocks: uint64(cd.size >> 8),
		Ordered: boolU64(cd.ordered), MaxTrial: uint64(cd.maxTrial),
		HeadFull: int64(cd.blocksHeadFull), HeadClosed: int64(cd.blocksHeadClosed),
		HeadOpen: int64(cd.blocksHeadOpen),
	}
	w.cedarHeader = &h
	w.array = cd.array[:cd.size]
	w.nInfos = cd.nInfos[:cd.size]
	w.blocks = cd.blocks[:cd.size>>8]
	if cd.Reduced {
		w.flags |= FlagReduced
	}
	return w.save(path)
}

// OpenSnapshot opens a cedar-only snapshot. Files containing AC links are
// rejected; use aho.OpenSnapshot for those.
func OpenSnapshot(path string) (*Snapshot, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, unmap, err := mmapFile(f)
	if err != nil {
		return nil, err
	}
	s, err := parseSnapshot(data, unmap, 0)
	if err != nil {
		_ = unmap()
		return nil, err
	}
	return s, nil
}

func boolU64(v bool) uint64 {
	if v {
		return 1
	}
	return 0
}

type snapshotWriter struct {
	flags       uint64
	size        int
	cedarHeader *snapshotCedarHeader
	array       []Node
	nInfos      []NInfo
	blocks      []Block
	fails       []int32
	outputs     []int32
	depth       []int32
}

func newSnapshotWriter(size int, flags uint64) (*snapshotWriter, error) {
	if size < 256 || size%256 != 0 || size>>8 > int(^uint32(0)>>1) {
		return nil, fmt.Errorf("%w: bad size %d", ErrInvalidSnapshot, size)
	}
	return &snapshotWriter{flags: flags, size: size}, nil
}

func (w *snapshotWriter) save(path string) error {
	hasAho := w.flags&FlagAhoLinks != 0
	nsec := 4
	if hasAho {
		nsec = 7
	}
	pos := fileHeaderSize + nsec*sectionSize
	pos = align8(pos + cedarHeaderSize)
	sec := make([]snapshotSection, nsec)
	set := func(i int, kind uint64, nbytes, elems int) {
		sec[i] = snapshotSection{Kind: kind, Offset: uint64(pos), Size: uint64(nbytes), Elements: uint64(elems)}
		pos = align8(pos + nbytes)
	}
	set(0, SectionCedarHeader, cedarHeaderSize, 0)
	set(1, SectionArray, w.size*8, w.size)
	set(2, SectionNInfo, w.size*2, w.size)
	set(3, SectionBlock, (w.size>>8)*24, w.size>>8)
	if hasAho {
		set(4, SectionFails, w.size*4, w.size)
		set(5, SectionOutputs, w.size*4, w.size)
		set(6, SectionDepth, w.size*4, w.size)
	}
	footerAt := pos
	total := footerAt + footerSize

	header := make([]byte, fileHeaderSize+nsec*sectionSize+cedarHeaderSize)
	fh := snapshotFileHeader{
		Magic: snapshotMagic, Version: SnapshotVersion,
		ByteOrder: 0x0102030405060708, Flags: w.flags, Sections: uint64(nsec),
	}
	putFileHeader(header, fh)
	for i, s := range sec {
		putSection(header[fileHeaderSize+i*sectionSize:], s)
	}
	putCedarHeader(header[fileHeaderSize+nsec*sectionSize:], *w.cedarHeader)

	dataCRC := crc32.NewIEEE()
	data := sectionData{array: w.array, nInfos: w.nInfos, blocks: w.blocks,
		fails: w.fails, outputs: w.outputs, depth: w.depth}
	crcSections(dataCRC, sec, data)

	footer := make([]byte, footerSize)
	putFooter(footer, snapshotFooter{
		Magic: snapshotMagic, HeaderCRC: uint64(crc32.ChecksumIEEE(header)), DataCRC: uint64(dataCRC.Sum32()),
	})

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = tmp.Close(); _ = os.Remove(tmpName) }
	if err := writeAll(tmp, header); err != nil {
		cleanup()
		return err
	}
	if err := writeSections(tmp, header, sec, data); err != nil {
		cleanup()
		return err
	}
	if err := writeAll(tmp, footer); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	_ = total
	return nil
}

func writeAll(f *os.File, b []byte) error {
	_, err := f.Write(b)
	return err
}

func sectionBytes(sec snapshotSection, data sectionData) []byte {
	switch sec.Kind {
	case SectionArray:
		return unsafe.Slice((*byte)(unsafe.Pointer(&data.array[0])), len(data.array)*8)
	case SectionNInfo:
		return unsafe.Slice((*byte)(unsafe.Pointer(&data.nInfos[0])), len(data.nInfos)*2)
	case SectionBlock:
		return unsafe.Slice((*byte)(unsafe.Pointer(&data.blocks[0])), len(data.blocks)*24)
	case SectionFails:
		return unsafe.Slice((*byte)(unsafe.Pointer(&data.fails[0])), len(data.fails)*4)
	case SectionOutputs:
		return unsafe.Slice((*byte)(unsafe.Pointer(&data.outputs[0])), len(data.outputs)*4)
	case SectionDepth:
		return unsafe.Slice((*byte)(unsafe.Pointer(&data.depth[0])), len(data.depth)*4)
	}
	return nil
}

func crcSections(h hash.Hash32, sec []snapshotSection, data sectionData) {
	for _, s := range sec {
		if s.Kind == SectionCedarHeader {
			continue
		}
		_, _ = h.Write(sectionBytes(s, data))
	}
}

func writeSections(f *os.File, header []byte, sec []snapshotSection, data sectionData) error {
	written := len(header)
	for _, s := range sec {
		for written < int(s.Offset) {
			if _, err := f.Write([]byte{0}); err != nil {
				return err
			}
			written++
		}
		b := sectionBytes(s, data)
		if _, err := f.Write(b); err != nil {
			return err
		}
		written += len(b)
	}
	return nil
}

func parseSnapshot(data []byte, unmap func() error, required uint64) (*Snapshot, error) {
	if len(data) < fileHeaderSize+footerSize {
		return nil, fmt.Errorf("%w: too short", ErrInvalidSnapshot)
	}
	fh := getFileHeader(data)
	if fh.Magic != snapshotMagic {
		return nil, fmt.Errorf("%w: bad magic", ErrInvalidSnapshot)
	}
	if fh.Version != SnapshotVersion {
		return nil, fmt.Errorf("%w: version %d", ErrSnapshotVersion, fh.Version)
	}
	if fh.ByteOrder != 0x0102030405060708 {
		return nil, fmt.Errorf("%w: unsupported byte order", ErrInvalidSnapshot)
	}
	if fh.Sections == 0 || fh.Sections > 7 {
		return nil, fmt.Errorf("%w: section count", ErrInvalidSnapshot)
	}
	if required != 0 && fh.Flags&FlagAhoLinks != required {
		return nil, fmt.Errorf("%w: AC links mismatch", ErrInvalidSnapshot)
	}
	if required == 0 && fh.Flags&FlagAhoLinks != 0 {
		return nil, fmt.Errorf("%w: cedar snapshot has AC links", ErrInvalidSnapshot)
	}

	footerOff := len(data) - footerSize
	footer := getFooter(data[footerOff:])
	if footer.Magic != snapshotMagic {
		return nil, fmt.Errorf("%w: bad footer", ErrInvalidSnapshot)
	}
	headerEnd := fileHeaderSize + int(fh.Sections)*sectionSize
	sections := make(map[uint64]snapshotSection, fh.Sections)
	for i := 0; i < int(fh.Sections); i++ {
		s := getSection(data[fileHeaderSize+i*sectionSize:])
		if _, exists := sections[s.Kind]; exists {
			return nil, fmt.Errorf("%w: duplicate section", ErrInvalidSnapshot)
		}
		if s.Offset < uint64(headerEnd) || int64(s.Offset) < 0 || s.Offset+s.Size > uint64(len(data)-footerSize) || s.Offset%8 != 0 {
			return nil, fmt.Errorf("%w: section bounds", ErrInvalidSnapshot)
		}
		sections[s.Kind] = s
	}

	need := []uint64{SectionCedarHeader, SectionArray, SectionNInfo, SectionBlock}
	if fh.Flags&FlagAhoLinks != 0 {
		need = append(need, SectionFails, SectionOutputs, SectionDepth)
	}
	for _, k := range need {
		if _, ok := sections[k]; !ok {
			return nil, fmt.Errorf("%w: missing section %d", ErrInvalidSnapshot, k)
		}
	}

	headerLen := headerEnd + cedarHeaderSize
	if headerLen > footerOff || footer.HeaderCRC != uint64(crc32.ChecksumIEEE(data[:headerLen])) {
		return nil, fmt.Errorf("%w: header CRC", ErrInvalidSnapshot)
	}
	hcrc := crc32.NewIEEE()
	for _, k := range []uint64{SectionArray, SectionNInfo, SectionBlock, SectionFails, SectionOutputs, SectionDepth} {
		if s, ok := sections[k]; ok {
			_, _ = hcrc.Write(data[s.Offset : s.Offset+s.Size])
		}
	}
	if hcrc.Sum32() != uint32(footer.DataCRC) {
		return nil, fmt.Errorf("%w: data CRC", ErrInvalidSnapshot)
	}

	ch := getCedarHeader(data[headerEnd:])
	if ch.Size < 256 || ch.Size%256 != 0 || ch.Capacity != ch.Size || ch.NumBlocks != ch.Size>>8 {
		return nil, fmt.Errorf("%w: cedar header", ErrInvalidSnapshot)
	}
	as := sections[SectionArray]
	ns := sections[SectionNInfo]
	bs := sections[SectionBlock]
	if as.Size != ch.Size*8 || as.Elements != ch.Size || ns.Size != ch.Size*2 || ns.Elements != ch.Size || bs.Size != ch.NumBlocks*24 || bs.Elements != ch.NumBlocks {
		return nil, fmt.Errorf("%w: section lengths", ErrInvalidSnapshot)
	}

	cd := &Cedar{
		Reduced:        fh.Flags&FlagReduced != 0,
		array:          unsafe.Slice((*Node)(unsafe.Pointer(&data[as.Offset])), ch.Size),
		nInfos:         unsafe.Slice((*NInfo)(unsafe.Pointer(&data[ns.Offset])), ch.Size),
		blocks:         unsafe.Slice((*Block)(unsafe.Pointer(&data[bs.Offset])), ch.NumBlocks),
		reject:         defaultReject(),
		blocksHeadFull: int32(ch.HeadFull), blocksHeadClosed: int32(ch.HeadClosed), blocksHeadOpen: int32(ch.HeadOpen),
		capacity: int(ch.Size), size: int(ch.Size), ordered: ch.Ordered != 0, maxTrial: int32(ch.MaxTrial),
	}
	if err := validateTrie(cd); err != nil {
		return nil, err
	}
	return &Snapshot{data: data, unmap: unmap, cd: cd, sections: sections, flags: fh.Flags}, nil
}

func defaultReject() [257]int32 {
	var r [257]int32
	for i := range r {
		r[i] = int32(i + 1)
	}
	return r
}

func validateTrie(cd *Cedar) error {
	if cd.size == 0 || len(cd.array) < cd.size || len(cd.nInfos) < cd.size || len(cd.blocks) < cd.size>>8 {
		return fmt.Errorf("%w: trie bounds", ErrInvalidSnapshot)
	}
	if cd.blocksHeadFull < 0 || cd.blocksHeadFull >= int32(len(cd.blocks)) ||
		cd.blocksHeadClosed < 0 || cd.blocksHeadClosed >= int32(len(cd.blocks)) ||
		cd.blocksHeadOpen < 0 || cd.blocksHeadOpen >= int32(len(cd.blocks)) {
		return fmt.Errorf("%w: block heads", ErrInvalidSnapshot)
	}
	for i := 0; i < cd.size; i++ {
		check := int(cd.array[i].Check)
		if check < 0 && check >= cd.size {
			return fmt.Errorf("%w: node check at %d", ErrInvalidSnapshot, i)
		}
	}
	return nil
}

// AhoLinks contains the three matcher arrays persisted in an aho snapshot.
type AhoLinks struct {
	Fails   []int32
	Outputs []int32
	Depth   []int32
}

// SaveSnapshotFile writes a cedar snapshot with optional AC link sections.
// It is the shared implementation behind cedar and aho snapshot APIs.
func (cd *Cedar) SaveSnapshotFile(path string, fails, outputs, depth []int32) error {
	flags := uint64(0)
	if cd.Reduced {
		flags |= FlagReduced
	}
	hasAho := fails != nil || outputs != nil || depth != nil
	if hasAho {
		if len(fails) != cd.size || len(outputs) != cd.size || len(depth) != cd.size {
			return fmt.Errorf("%w: AC link lengths", ErrInvalidSnapshot)
		}
		flags |= FlagAhoLinks
	}
	w, err := newSnapshotWriter(cd.size, flags)
	if err != nil {
		return err
	}
	w.cedarHeader = &snapshotCedarHeader{
		Size: uint64(cd.size), Capacity: uint64(cd.size), NumBlocks: uint64(cd.size >> 8),
		Ordered: boolU64(cd.ordered), MaxTrial: uint64(cd.maxTrial),
		HeadFull: int64(cd.blocksHeadFull), HeadClosed: int64(cd.blocksHeadClosed), HeadOpen: int64(cd.blocksHeadOpen),
	}
	w.array, w.nInfos, w.blocks = cd.array[:cd.size], cd.nInfos[:cd.size], cd.blocks[:cd.size>>8]
	w.fails, w.outputs, w.depth = fails, outputs, depth
	return w.save(path)
}

// OpenSnapshotFile opens a snapshot from an already-open file. required should
// be 0 for cedar snapshots or FlagAhoLinks for matcher snapshots.
func OpenSnapshotFile(path string, f *os.File, required uint64) (*Snapshot, error) {
	data, unmap, err := mmapFile(f)
	if err != nil {
		return nil, err
	}
	s, err := parseSnapshot(data, unmap, required)
	if err != nil {
		_ = unmap()
		return nil, err
	}
	return s, nil
}

// AhoLinks returns persisted AC arrays. It returns nil arrays for cedar-only
// snapshots.
func (s *Snapshot) AhoLinks() AhoLinks {
	if s.flags&FlagAhoLinks == 0 {
		return AhoLinks{}
	}
	f := s.sections[SectionFails]
	o := s.sections[SectionOutputs]
	d := s.sections[SectionDepth]
	n := int(s.cd.size)
	return AhoLinks{
		Fails:   unsafe.Slice((*int32)(unsafe.Pointer(&s.data[f.Offset])), n),
		Outputs: unsafe.Slice((*int32)(unsafe.Pointer(&s.data[o.Offset])), n),
		Depth:   unsafe.Slice((*int32)(unsafe.Pointer(&s.data[d.Offset])), n),
	}
}
