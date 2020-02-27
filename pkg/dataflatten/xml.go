package dataflatten

import (
	"io/ioutil"

	"github.com/clbanning/mxj"
	"github.com/pkg/errors"
)

func XmlFile(file string, opts ...FlattenOpts) ([]Row, error) {
	rawdata, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return Xml(rawdata, opts...)
}

func Xml(rawdata []byte, opts ...FlattenOpts) ([]Row, error) {
	mv, err := mxj.NewMapXml(rawdata)

	if err != nil {
		return nil, errors.Wrap(err, "mxj parse")
	}

	return Flatten(mv.Old())
}
