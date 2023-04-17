package types

import "go.etcd.io/bbolt"

type BboltDB interface {
	BboltDB() *bbolt.DB
}
