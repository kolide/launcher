package dataflatten

import (
	"bufio"
	"fmt"
	"os"

	"github.com/clbanning/mxj"
)

func XmlFile(file string, opts ...FlattenOpts) ([]Row, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Wrap the file in a bufio.Reader before handing it to mxj. mxj wraps any
	// reader that isn't an io.ByteReader in a helper whose ReadByte issues a
	// separate one-byte read syscall per byte; using bufio.Reader here avoids these
	// extra syscalls.
	rdr := bufio.NewReader(f)

	mv, err := mxj.NewMapXmlReader(rdr)
	if err != nil {
		return nil, err
	}

	return Flatten(mv.Old(), opts...)
}

func Xml(rawdata []byte, opts ...FlattenOpts) ([]Row, error) {
	mv, err := mxj.NewMapXml(rawdata)

	if err != nil {
		return nil, fmt.Errorf("mxj parse: %w", err)
	}

	return Flatten(mv.Old(), opts...)
}
