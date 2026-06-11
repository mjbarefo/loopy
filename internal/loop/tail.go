package loop

import (
	"bytes"
	"io"
	"os"
)

// ViewerCapBytes caps how much of any artifact a viewer loads. Drill-down
// viewers are tail-first: the last 256 KiB with a truncation banner, never
// the whole file.
const ViewerCapBytes = 256 * 1024

// TailFile reads at most max bytes from the end of path. size is the file's
// full size and truncated reports whether earlier content was dropped; a
// truncated read starts at the first complete line so viewers never show a
// torn first row. A missing file returns os.ErrNotExist.
func TailFile(path string, max int64) (data []byte, truncated bool, size int64, err error) {
	if max <= 0 {
		max = ViewerCapBytes
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, false, 0, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, false, 0, err
	}
	size = info.Size()
	if size <= max {
		data, err = io.ReadAll(f)
		return data, false, size, err
	}
	if _, err := f.Seek(-max, io.SeekEnd); err != nil {
		return nil, false, size, err
	}
	data, err = io.ReadAll(f)
	if err != nil {
		return nil, false, size, err
	}
	if i := bytes.IndexByte(data, '\n'); i >= 0 && i+1 < len(data) {
		data = data[i+1:]
	}
	return data, true, size, nil
}
