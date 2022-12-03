package writefstest

import (
	"bytes"
	"io"
	"io/fs"
	"os"
)

type (
	// MapFS is an in memory [xorkevin.dev/governor/util/writefs.FS]
	MapFS map[string]*MapFile

	// MapFile is an in memory file
	MapFile struct {
		Data []byte
		Mode fs.FileMode
	}
)

const (
	rwFlagMask = os.O_RDONLY | os.O_WRONLY | os.O_RDWR
)

func isReadWrite(flag int) (bool, bool) {
	switch flag & rwFlagMask {
	case os.O_RDONLY:
		return true, false
	case os.O_WRONLY:
		return false, true
	case os.O_RDWR:
		return true, true
	default:
		return false, false
	}
}

func (m MapFS) OpenFile(name string, flag int, mode fs.FileMode) (io.ReadWriteCloser, error) {
	isRead, isWrite := isReadWrite(flag)
	if isRead && isWrite {
		// do not support both reading and writing for simplicity
		return nil, fs.ErrInvalid
	}

	if flag&os.O_CREATE == 0 {
		if !isWrite {
			// disallow create when not writing
			return nil, fs.ErrInvalid
		}
		if flag&os.O_EXCL != 0 {
			// disallow using excl when create not specified
			return nil, fs.ErrInvalid
		}
	}

	f := m[name]
	if f == nil {
		if flag&os.O_CREATE == 0 {
			return nil, fs.ErrNotExist
		}

		f = &MapFile{
			Data: nil,
			Mode: mode,
		}
	} else {
		if flag&os.O_EXCL != 0 {
			return nil, fs.ErrExist
		}
	}

	data := f.Data
	end := false
	if flag&os.O_TRUNC != 0 {
		if !isWrite {
			// disallow using trunc when not writing
			return nil, fs.ErrInvalid
		}
		data = nil
	}
	if flag&os.O_APPEND != 0 {
		if !isWrite {
			// disallow using append when not writing
			return nil, fs.ErrInvalid
		}
		end = true
	}

	var r *bytes.Reader
	if isRead {
		r = bytes.NewReader(data)
	}
	var b *bytes.Buffer
	if isWrite {
		b = &bytes.Buffer{}
		if end {
			b.Write(data)
		}
	}

	return &mapFileReadWriter{
		r:    r,
		b:    b,
		mode: f.Mode,
		name: name,
		fsys: m,
	}, nil
}

type (
	mapFileReadWriter struct {
		r    *bytes.Reader
		b    *bytes.Buffer
		mode fs.FileMode
		name string
		fsys MapFS
	}
)

func (w *mapFileReadWriter) Read(b []byte) (int, error) {
	if w.r == nil {
		return 0, fs.ErrInvalid
	}
	return w.r.Read(b)
}

func (w *mapFileReadWriter) Write(p []byte) (int, error) {
	if w.b == nil {
		return 0, fs.ErrInvalid
	}
	return w.b.Write(p)
}

func (w *mapFileReadWriter) Close() error {
	if w.b != nil {
		w.fsys[w.name] = &MapFile{
			Data: w.b.Bytes(),
			Mode: w.mode,
		}
		w.b = nil
	}
	return nil
}
