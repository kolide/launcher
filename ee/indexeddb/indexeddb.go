package indexeddb

import (
	"encoding/binary"
	"fmt"
	"strings"

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
	objectStoreId := 0
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
			objectStoreId = i
			break
		}
	}

	fmt.Println(objectStoreId)

	// Now, we can read all records in this object store.
	objs := make([]map[string]any, 0)
	iter := db.NewIterator(nil, nil)
	for iter.Next() {
		key := iter.Key()
		value := iter.Value()

		if strings.Contains(string(key), "signinAddress") || strings.Contains(string(value), "signinAddress") {
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
