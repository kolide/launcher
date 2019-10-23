package dataflatten

import (
	"encoding/json"

	"github.com/pkg/errors"
)

func Json(rawdata []byte) ([]Row, error) {
	var data interface{}

	if err := json.Unmarshal(rawdata, &data); err != nil {
		return nil, errors.Wrap(err, "unmarshalling json")
	}

	return Flatten(data)
}
