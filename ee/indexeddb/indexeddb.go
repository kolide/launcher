package indexeddb

import (
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

func QueryIndexeddb(dbLocation string, dbName string, objectStoreName string) ([]map[string]any, error) {
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
	fmt.Println(databaseId)

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
