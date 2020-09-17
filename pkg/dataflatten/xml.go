package dataflatten

import (
	"os"

	"github.com/clbanning/mxj"
	"github.com/pkg/errors"
)

func XmlFile(file string, opts ...FlattenOpts) ([]Row, error) {
	rdr, err := os.Open(file)
	if err != nil {
		return nil, err
	}

	mv, err := mxj.NewMapXmlReader(rdr)
	if err != nil {
		return nil, err
	}

	return Flatten(mv.Old(), opts...)
}

func Xml(rawdata []byte, opts ...FlattenOpts) ([]Row, error) {
	mv, err := mxj.NewMapXml(rawdata)

	if err != nil {
		return nil, errors.Wrap(err, "mxj parse")
	}

	return Flatten(mv.Old(), opts...)
}
