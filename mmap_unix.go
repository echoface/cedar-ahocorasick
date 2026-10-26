//go:build unix

package cedar

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// mmapFile maps an existing read-only file and returns its bytes together with
// an unmap function.
func mmapFile(f *os.File) ([]byte, func() error, error) {
	st, err := f.Stat()
	if err != nil {
		return nil, nil, err
	}
	if st.Size() == 0 {
		return nil, func() error { return nil }, nil
	}
	n := int(st.Size())
	b, err := unix.Mmap(int(f.Fd()), 0, n, unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		return nil, nil, fmt.Errorf("cedar: mmap: %w", err)
	}
	unmap := func() error {
		if len(b) == 0 {
			return nil
		}
		if err := unix.Munmap(b); err != nil {
			return fmt.Errorf("cedar: munmap: %w", err)
		}
		return nil
	}
	// Keep the byte slice alive for unsafe.Slice views until unmap is called.
	return b, unmap, nil
}
