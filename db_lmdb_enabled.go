//go:build lmdb

package main

import (
	"github.com/fiatjaf/eventstore/lmdb"
)

// newLMDBBackend is compiled only when the "lmdb" build tag is set.
func newLMDBBackend(path string) DBBackend {
	return &lmdb.LMDBBackend{Path: path}
}
