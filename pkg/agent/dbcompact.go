package agent

import (
	"errors"
	"fmt"
	"os"
	"time"

	"go.etcd.io/bbolt"
)

var (
	boltOptions = &bbolt.Options{Timeout: time.Duration(10) * time.Second}
)

// DbCompact provides a wrapper around boltdb's compaction routine. This
// works by opening a new database and rewriting all the data.This
// requires nothing be using the database, and requires some manual file
// moving around.
func DbCompact(boltPath string, compactMaxTxSize int64) (string, error) {
	if !fileExists(boltPath) {
		return "", errors.New("No launcher.db. Cannot compact")
	}

	newBoltPath := boltPath + ".new"
	oldBoltPath := boltPath + ".old"

	if err := compact(boltPath, newBoltPath, compactMaxTxSize); err != nil {
		return "", err
	}

	if err := rename(boltPath, newBoltPath, oldBoltPath); err != nil {
		return "", err
	}

	return oldBoltPath, nil
}

func compact(boltPath, newBoltPath string, compactMaxTxSize int64) error {
	// compact is a janky re-write operation. So we need to ensure no one has the DB open
	src, err := bbolt.Open(boltPath, 0444, boltOptions)
	if err != nil {
		return fmt.Errorf("unable to open existing launcher.db. Perhaps launcher is still running?: %w", err)
	}
	defer src.Close()

	dst, err := bbolt.Open(newBoltPath, 0600, boltOptions)
	if err != nil {
		return fmt.Errorf("open new launcher.db: %w", err)
	}
	defer dst.Close()

	if err := bbolt.Compact(dst, src, compactMaxTxSize); err != nil {
		return fmt.Errorf("compacting database: %w", err)
	}

	return nil
}

func rename(boltPath, newBoltPath, oldBoltPath string) error {
	if err := os.Rename(boltPath, oldBoltPath); err != nil {
		return fmt.Errorf("moving launcher.db to launcher.db.old: %w", err)
	}

	if err := os.Rename(newBoltPath, boltPath); err != nil {
		return fmt.Errorf("moving launcher.db.new to launcher.db: %w", err)
	}

	return nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}
