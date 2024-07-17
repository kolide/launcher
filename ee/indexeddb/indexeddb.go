// Package indexeddb provides the ability to query an IndexedDB and parse the objects it contains.
package indexeddb

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/kolide/goleveldb/leveldb"
	"github.com/kolide/goleveldb/leveldb/opt"
	"github.com/kolide/launcher/ee/agent"
)

// maxNumberOfObjectStoresToCheck is the number of indices for object stores we will check
// before declaring failure to find the given object store. We cannot look up
// object stores by their names, only by their IDs -- so we have to iterate through
// up to maxNumberOfObjectStoresToCheck IDs to find the desired store. We assume there are
// fewer than 100 object stores in a given database. (We may need to adjust this
// number upward after further research, but for now this seems like a safe upper
// bounds.)
const maxNumberOfObjectStoresToCheck = 100

// indexeddbComparer can be used when opening any IndexedDB instance.
var indexeddbComparer = newChromeComparer()

// QueryIndexeddbObjectStore queries the indexeddb at the given location `dbLocation`,
// returning all objects in the given database that live in the given object store.
func QueryIndexeddbObjectStore(dbLocation string, dbName string, objectStoreName string) ([]map[string][]byte, error) {
	// If Chrome is open, we won't be able to open the db. So, copy it to a temporary location first.
	tempDbCopyLocation, err := copyIndexeddb(dbLocation)
	if err != nil {
		if tempDbCopyLocation != "" {
			_ = os.RemoveAll(tempDbCopyLocation)
		}
		return nil, fmt.Errorf("unable to copy db: %w", err)
	}
	// The copy was successful -- make sure we clean it up after we're done
	defer os.RemoveAll(tempDbCopyLocation)

	opts := &opt.Options{
		Comparer:               indexeddbComparer,
		DisableSeeksCompaction: true, // no need to perform compaction
		Strict:                 opt.StrictAll,
	}
	db, err := leveldb.OpenFile(tempDbCopyLocation, opts)
	if err != nil {
		// Perform recover in case we missed something while copying
		db, err = leveldb.RecoverFile(tempDbCopyLocation, opts)
	}

	if err != nil {
		return nil, fmt.Errorf("opening db: %w", err)
	}
	defer db.Close()

	// Get the database ID from the name
	databaseNameKey, err := databaseIdKey(dbLocation, dbName)
	if err != nil {
		return nil, fmt.Errorf("getting database id key: %w", err)
	}
	databaseIdRaw, err := db.Get(databaseNameKey, nil)
	if err != nil {
		return nil, fmt.Errorf("querying for database id: %w", err)
	}
	databaseId, _ := binary.Uvarint(databaseIdRaw)

	// We can't query for the object store ID by its name -- we have to query each ID to get its name,
	// and check against that. Object store indices start at 1.
	var objectStoreId uint64
	var i uint64
	for i = 1; i <= maxNumberOfObjectStoresToCheck; i++ {
		objectStoreNameRaw, err := db.Get(objectStoreNameKey(databaseId, i), nil)
		if err != nil {
			continue
		}
		foundObjectStoreName, err := decodeUtf16BigEndianBytes(objectStoreNameRaw)
		if err != nil {
			continue
		}
		if string(foundObjectStoreName) == objectStoreName {
			objectStoreId = i
			break
		}
	}

	if objectStoreId == 0 {
		return nil, errors.New("unable to get object store ID")
	}

	// Get the key prefix for all objects in this store.
	keyPrefix := objectDataKeyPrefix(databaseId, objectStoreId)

	// Now, we can read all records, keeping only the ones with our matching key prefix.
	objs := make([]map[string][]byte, 0)
	iter := db.NewIterator(nil, nil)
	for iter.Next() {
		key := iter.Key()
		if !bytes.HasPrefix(key, keyPrefix) {
			continue
		}

		tmp := make([]byte, len(iter.Value()))
		copy(tmp, iter.Value())
		objs = append(objs, map[string][]byte{
			"data": tmp,
		})
	}
	iter.Release()
	if err := iter.Error(); err != nil {
		return objs, fmt.Errorf("iterator error: %w", err)
	}

	return objs, nil
}

func copyIndexeddb(sourceDb string) (string, error) {
	dbCopyLocation, err := agent.MkdirTemp(filepath.Base(sourceDb))
	if err != nil {
		return "", fmt.Errorf("making temporary directory: %w", err)
	}

	entries, err := os.ReadDir(sourceDb)
	if err != nil {
		return dbCopyLocation, fmt.Errorf("reading directory contents: %w", err)
	}
	for _, entry := range entries {
		// We expect only files in the database -- no directories, symlinks, etc.
		// Ignore any unexpected files.
		if entry.IsDir() || !entry.Type().IsRegular() {
			continue
		}
		// We don't want to copy over the lock -- we won't be able to open it for reading.
		if entry.Name() == "LOCK" {
			continue
		}
		src := filepath.Join(sourceDb, entry.Name())
		dest := filepath.Join(dbCopyLocation, entry.Name())
		if err := copyFile(src, dest); err != nil {
			return dbCopyLocation, fmt.Errorf("copying file: %w", err)
		}
	}

	return dbCopyLocation, nil
}

func copyFile(src string, dest string) error {
	srcFh, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening %s: %w", src, err)
	}
	defer srcFh.Close()

	destFh, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("opening %s: %w", dest, err)
	}

	if _, err := io.Copy(destFh, srcFh); err != nil {
		_ = destFh.Close()
		return fmt.Errorf("copying %s to %s: %w", src, dest, err)
	}

	if err := destFh.Close(); err != nil {
		return fmt.Errorf("completing write from %s to %s: %w", src, dest, err)
	}

	return nil
}
