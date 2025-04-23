// Package indexeddb provides the ability to query an IndexedDB and parse the objects it contains.
package indexeddb

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/kolide/goleveldb/leveldb"
	leveldberrors "github.com/kolide/goleveldb/leveldb/errors"
	"github.com/kolide/goleveldb/leveldb/opt"
	"github.com/kolide/launcher/ee/agent"
	"github.com/kolide/launcher/ee/observability"
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
func QueryIndexeddbObjectStore(ctx context.Context, dbLocation string, dbName string, objectStoreName string) ([]map[string][]byte, error) {
	ctx, span := observability.StartSpan(ctx, "db_name", dbName, "object_store_name", objectStoreName)
	defer span.End()

	// If Chrome is open, we won't be able to open the db. So, copy it to a temporary location first.
	tempDbCopyLocation, err := CopyLeveldb(ctx, dbLocation)
	if err != nil {
		if tempDbCopyLocation != "" {
			_ = os.RemoveAll(tempDbCopyLocation)
		}
		return nil, fmt.Errorf("unable to copy db: %w", err)
	}
	// The copy was successful -- make sure we clean it up after we're done
	defer os.RemoveAll(tempDbCopyLocation)

	objs := make([]map[string][]byte, 0)

	db, err := OpenLeveldb(tempDbCopyLocation)
	if err != nil {
		return nil, fmt.Errorf("opening leveldb: %w", err)
	}
	defer db.Close()
	span.AddEvent("db_opened")

	// Get the database ID from the name
	databaseNameKey, err := databaseIdKey(dbLocation, dbName)
	if err != nil {
		return nil, fmt.Errorf("getting database id key: %w", err)
	}
	databaseIdRaw, err := db.Get(databaseNameKey, nil)
	if err != nil {
		// If the database doesn't exist, return an empty list of objects
		if errors.Is(err, leveldberrors.ErrNotFound) {
			return objs, nil
		}
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
	span.AddEvent("got_object_store_id")

	if objectStoreId == 0 {
		// If the object store doesn't exist, return an empty list of objects
		return objs, nil
	}

	// Get the key prefix for all objects in this store.
	keyPrefix := objectDataKeyPrefix(databaseId, objectStoreId)

	// Now, we can read all records, keeping only the ones with our matching key prefix.
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
	span.AddEvent("completed_iteration")

	return objs, nil
}

func CopyLeveldb(ctx context.Context, sourceDb string) (string, error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

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
		if err := copyFile(ctx, src, dest); err != nil {
			return dbCopyLocation, fmt.Errorf("copying file: %w", err)
		}
	}

	return dbCopyLocation, nil
}

func copyFile(ctx context.Context, src string, dest string) error {
	_, span := observability.StartSpan(ctx)
	defer span.End()

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

func OpenLeveldb(dbLocation string) (*leveldb.DB, error) {
	opts := &opt.Options{
		Comparer:               indexeddbComparer,
		DisableSeeksCompaction: true,               // no need to perform compaction
		Strict:                 opt.StrictRecovery, // we prefer to drop corrupted data rather than fail to open the db altogether
	}
	db, dbOpenErr := leveldb.OpenFile(dbLocation, opts)
	if dbOpenErr != nil {
		// Perform recover in case we missed something while copying
		var dbRecoverErr error
		db, dbRecoverErr = leveldb.RecoverFile(dbLocation, opts)
		if dbRecoverErr != nil {
			return nil, fmt.Errorf("opening db: `%+v`; recovering db: %w", dbOpenErr, dbRecoverErr)
		}
	}

	return db, nil
}
