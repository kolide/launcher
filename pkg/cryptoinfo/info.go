package cryptoinfo

import "encoding/json"

type KeyInfo struct {
	Type     string
	Encoding string
	Data     interface{}
	DataName string
	Error    error
	Headers  map[string]string
}

// Maybe make these types?
const (
	kiPEM         = "PEM"
	kiDER         = "DER"
	kiCertificate = "CERTIFICATE"
)

func NewKeyInfo(typ, encoding string, headers map[string]string) *KeyInfo {
	return &KeyInfo{
		Type:     typ,
		Encoding: encoding,
		Headers:  headers,
	}

}

func (ki *KeyInfo) SetDataName(name string) *KeyInfo {
	ki.DataName = name
	return ki
}

func (ki *KeyInfo) SetData(data interface{}, err error) *KeyInfo {
	ki.Data = data
	ki.Error = err
	return ki
}

func (ki *KeyInfo) MarshalJSON() ([]byte, error) {
	// this feels somewhat inefficient WRT to allocations and shoving maps around. But it
	// also feels the simplest way to get consistent behavior without needing to push
	// the key/value pairs everywhere.
	ret := map[string]interface{}{
		"type":     ki.Type,
		"encoding": ki.Encoding,
	}

	if ki.Error != nil {
		ret["error"] = ki.Error.Error()
	} else {
		if ki.DataName != "" {
			ret[ki.DataName] = ki.Data
		} else {
			ret["error"] = "No data name"
		}
	}

	if len(ki.Headers) != 0 {
		ret["headers"] = ki.Headers
	}

	return json.Marshal(ret)
}
