package indexeddb

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

func QueryIndexeddbObjectStore(dbLocation string, dbName string, objectStoreName string) ([]map[string]any, error) {
	opts := &opt.Options{
		Comparer: &chromeComparer{},
	}

	db, err := leveldb.OpenFile(dbLocation, opts)
	if err != nil {
		return nil, fmt.Errorf("opening db: %w", err)
	}
	defer db.Close()

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
	for i := 1; i < 100; i++ {
		objectStoreNameRaw, err := db.Get(objectStoreNameKey(databaseId, uint64(i)), nil)
		if err != nil {
			continue
		}
		foundObjectStoreName, err := utf16BigEndianBytesToString(objectStoreNameRaw)
		if err != nil {
			continue
		}
		if string(foundObjectStoreName) == objectStoreName {
			objectStoreId = uint64(i)
			break
		}
	}

	keyPrefix := objectDataKeyPrefix(databaseId, objectStoreId)

	// Now, we can read all records, parsing only the ones with our matching key prefix.
	objs := make([]map[string]any, 0)
	iter := db.NewIterator(nil, nil)
	for iter.Next() {
		key := iter.Key()
		value := iter.Value()

		if bytes.HasPrefix(key, keyPrefix) {
			obj, err := valueDecode(value)
			if err != nil {
				return objs, fmt.Errorf("decoding object: %w", err)
			}

			objs = append(objs, obj)
		}
	}
	iter.Release()
	if err := iter.Error(); err != nil {
		return objs, fmt.Errorf("iterator error: %w", err)
	}

	return objs, nil
}
