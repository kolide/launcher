package types

import "go.etcd.io/bbolt"

// Kontext ("Kolide context") holds references to data and structs which are used throughout
// launcher code and are typically valid for the lifetime of the application instance.
type Kontext struct {
	Storage Storage

	// BboltDB is the underlying bbolt database.
	// Ideally, we can eventually remove this. This is only here because some parts of the codebase
	// like the osquery extension have a direct dependency on bbolt and need this reference.
	// If we are able to abstract bbolt out completely in these areas, we should be able to
	// remove this field and prevent "leaking" bbolt into places it doesn't need to.
	BboltDB *bbolt.DB

	// This struct is a work in progress, and will be iteratively added to as needs arise.
	// Some potential future additions include:
	// Flags
	// Querier
}

func NewKontext(s Storage, db *bbolt.DB) *Kontext {
	ktx := &Kontext{
		Storage: s,
		BboltDB: db,
	}

	return ktx
}
