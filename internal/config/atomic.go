package config

import (
	"os"
	"path/filepath"
)

// WriteFileAtomic0600 writes data to path via a same-directory temp file
// renamed over the target. The temp file gets mode 0600 before any byte
// of data is written, so secret content is never observable at a looser
// mode — os.WriteFile alone cannot guarantee that, because open(2)
// ignores the requested mode when the file already exists.
func WriteFileAtomic0600(path string, data []byte) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".vb-tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	fail := func(err error) error {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Chmod(0o600); err != nil {
		return fail(err)
	}
	if _, err := f.Write(data); err != nil {
		return fail(err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
