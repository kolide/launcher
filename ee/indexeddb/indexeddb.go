package indexeddb

import (
	"fmt"
	"strings"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

func queryIndexeddb(dbLocation string) ([]map[string]any, error) {
	opts := &opt.Options{
		Comparer: &chromeComparer{},
	}

	db, err := leveldb.OpenFile(dbLocation, opts)
	if err != nil {
		return nil, fmt.Errorf("opening db: %w", err)
	}
	defer db.Close()

	objs := make([]map[string]any, 0)
	iter := db.NewIterator(nil, nil)
	for iter.Next() {
		key := iter.Key()
		value := iter.Value()

		if strings.Contains(string(key), "signinAddress") || strings.Contains(string(value), "signinAddress") {
			// TODO: parse key
			// https://github.com/chromium/chromium/blob/main/content/browser/indexed_db/docs/leveldb_coding_scheme.md

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
