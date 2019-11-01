package dataflatten

import (
	"encoding/json"
	"io/ioutil"

	"github.com/pkg/errors"
)

func JsonFile(file string, opts ...FlattenOpts) ([]Row, error) {
	rawdata, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return Json(rawdata, opts...)
}

func Json(rawdata []byte, opts ...FlattenOpts) ([]Row, error) {
	var data interface{}

	if err := json.Unmarshal(rawdata, &data); err != nil {
		return nil, errors.Wrap(err, "unmarshalling json")
	}

	return Flatten(data, opts...)
}
