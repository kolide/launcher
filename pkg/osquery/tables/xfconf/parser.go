//go:build !windows
// +build !windows

package xfconf

import (
	"encoding/xml"
	"fmt"
	"os"

	"github.com/kolide/launcher/pkg/dataflatten"
)

type (
	ArrayValue struct {
		Type  string `xml:"type,attr"`
		Value string `xml:"value,attr"`
	}
	Property struct {
		Name       string       `xml:"name,attr"`
		Type       string       `xml:"type,attr"`
		Value      string       `xml:"value,attr"`
		Properties []Property   `xml:"property"`
		Values     []ArrayValue `xml:"value"`
	}
	ChannelXML struct {
		XMLName     xml.Name   `xml:"channel"`
		ChannelName string     `xml:"name,attr"`
		Properties  []Property `xml:"property"`
	}
)

// parseXml reads in the given xml file, parses it as an xfconf XML file, and then flattens
// it. Because most XML elements in the xfconf files are `property`, we parse the file into
// a map with the name attributes set as the keys to avoid loss of meaningful full keys.
func parseXfconfXml(file string, opts ...dataflatten.FlattenOpts) ([]dataflatten.Row, error) {
	channelXml, err := readChannelXml(file)
	if err != nil {
		return nil, fmt.Errorf("could not read xfconf channel file %s: %w", file, err)
	}

	return dataflatten.Flatten(channelXml.toMap(), opts...)
}

func readChannelXml(file string) (ChannelXML, error) {
	rdr, err := os.Open(file)
	if err != nil {
		return ChannelXML{}, err
	}

	xmlDecoder := xml.NewDecoder(rdr)

	var result ChannelXML
	xmlDecoder.Decode(&result)

	return result, nil
}

// toMap transforms Result r into a map where the top-level key is "channel/<name>".
func (c ChannelXML) toMap() map[string]interface{} {
	parentKey := fmt.Sprintf("channel/%s", c.ChannelName)

	properties := make(map[string]interface{}, 0)
	for _, p := range c.Properties {
		properties[p.Name] = p.mapValue()
	}

	results := make(map[string]interface{})
	results[parentKey] = properties

	return results
}

func (p Property) mapValue() interface{} {
	var propertyValue interface{}
	if len(p.Properties) > 0 {
		childPropertyMaps := make(map[string]interface{}, 0)
		for _, child := range p.Properties {
			// Call recursively for each child
			childPropertyMaps[child.Name] = child.mapValue()
		}

		propertyValue = childPropertyMaps
	} else if p.Type == "array" {
		arrayValues := make([]interface{}, len(p.Values))
		for i, v := range p.Values {
			arrayValues[i] = v.Value
		}

		propertyValue = arrayValues
	} else {
		propertyValue = p.Value
	}

	return propertyValue
}
