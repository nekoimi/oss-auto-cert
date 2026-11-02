//go:build !windows

package storage

import "os"

func secureDir(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	return os.Chmod(path, 0700)
}
func restrictFile(string) error     { return nil }
func replace(old, new string) error { return os.Rename(old, new) }
