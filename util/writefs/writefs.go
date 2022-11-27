package writefs

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"xorkevin.dev/kerrors"
)

type (
	// FS is a file system that may be read from and written to
	FS interface {
		OpenFile(name string, flag int, mode fs.FileMode) (io.ReadWriteCloser, error)
	}

	// OSFS implements [FS] with the os file system
	OSFS struct {
		Base string
	}
)

// NewOSFS creates a new os write fs
func NewOSFS(base string) *OSFS {
	return &OSFS{
		Base: base,
	}
}

// OpenFile implements [FS]
//
// When O_CREATE is set, it will create any directories in the path of the file
// with 0777 (before umask)
func (o *OSFS) OpenFile(name string, flag int, mode fs.FileMode) (io.ReadWriteCloser, error) {
	if !fs.ValidPath(name) {
		return nil, fs.ErrInvalid
	}
	path := filepath.Join(o.Base, name)
	if flag&os.O_CREATE != 0 {
		if err := os.MkdirAll(filepath.Dir(path), 0777); err != nil {
			return nil, kerrors.WithMsg(err, "Failed to mkdir")
		}
	}
	f, err := os.OpenFile(path, flag, mode)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to open file")
	}
	return f, nil
}
