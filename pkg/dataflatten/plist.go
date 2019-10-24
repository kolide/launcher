package dataflatten

import (
	"github.com/groob/plist"
	"github.com/pkg/errors"
)

func Plist(rawdata []byte) ([]Row, error) {
	var data interface{}

	if err := plist.Unmarshal(rawdata, &data); err != nil {
		return nil, errors.Wrap(err, "unmarshalling plist")
	}

	return Flatten(data)
}
