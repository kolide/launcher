package dataflatten

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

func TomlFile(file string, opts ...FlattenOpts) ([]Row, error) {
	rawdata, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return Toml(rawdata, opts...)
}

func Toml(rawdata []byte, opts ...FlattenOpts) ([]Row, error) {
	var data map[string]any
	_, err := toml.Decode(string(rawdata), &data)
	if err != nil {
		return nil, fmt.Errorf("decoding toml: %w", err)
	}
	return Flatten(data, opts...)
}
