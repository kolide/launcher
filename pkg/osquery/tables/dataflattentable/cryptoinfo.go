package dataflattentable

import (
	"encoding/json"
	"os"

	"github.com/kolide/launcher/pkg/cryptoinfo"
	"github.com/kolide/launcher/pkg/dataflatten"
	"github.com/pkg/errors"
)

// flattenCryptoInfo is a small wrapper over pkg/cryptoinfo that passes it off to dataflatten for table generation
func flattenCryptoInfo(filename string, opts ...dataflatten.FlattenOpts) ([]dataflatten.Row, error) {
	filebytes, err := os.ReadFile(filename)
	if err != nil {
		return nil, errors.Wrapf(err, "reading %s", filename)
	}

	result, err := cryptoinfo.Identify(filebytes)
	if err != nil {
		return nil, errors.Wrap(err, "parsing with cryptoinfo")
	}

	// convert to json, so it's parsable
	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return nil, errors.Wrap(err, "json")
	}

	return dataflatten.Json(jsonBytes, opts...)
}
