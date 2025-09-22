//go:build !lmdb

package main

// newLMDBBackend is a stub used when the "lmdb" build tag is not set.
// If DB_ENGINE=lmdb is selected at runtime without the build tag, this
// will panic with a clear message so users know how to enable it.
func newLMDBBackend(path string) DBBackend {
	panic("LMDB backend not included in this build. Rebuild with -tags lmdb to enable LMDB support.")
}
