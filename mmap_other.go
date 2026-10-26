//go:build !unix

package cedar

import (
	"fmt"
	"io"
	"os"
)

func mmapFile(f *os.File) ([]byte, func() error, error) {
	st, err := f.Stat()
	if err != nil {
		return nil, nil, err
	}
	b := make([]byte, st.Size())
	if _, err := io.ReadFull(f, b); err != nil {
		return nil, nil, fmt.Errorf("cedar: read snapshot: %w", err)
	}
	return b, func() error { return nil }, nil
}
