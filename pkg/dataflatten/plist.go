package dataflatten

import (
	"io/ioutil"

	"github.com/groob/plist"
	"github.com/pkg/errors"
)

func PlistFile(file string, opts ...FlattenOpts) ([]Row, error) {
	rawdata, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return Plist(rawdata, opts...)
}

func Plist(rawdata []byte, opts ...FlattenOpts) ([]Row, error) {
	var data interface{}

	if err := plist.Unmarshal(rawdata, &data); err != nil {
		return nil, errors.Wrap(err, "unmarshalling plist")
	}

	return Flatten(data, opts...)
}
