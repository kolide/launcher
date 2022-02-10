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
	kiPEM = "PEM"
	kiDER = "DER"
	kiP12 = "P12"

	kiCACERTIFICATE = "CA-CERTIFICATE" // What is correct here?
	kiCaCertificate = "certificate"
	kiCERTIFICATE   = "CERTIFICATE"
	kiCertificate   = "certificate"
	kiKEY           = "KEY"
	kiKey           = "key"
)

func NewKIKey(encoding string) *KeyInfo {
	return &KeyInfo{
		DataName: kiKey,
		Encoding: encoding,
		Type:     kiKEY,
	}
}

func NewKICertificate(encoding string) *KeyInfo {
	return &KeyInfo{
		DataName: kiCertificate,
		Encoding: encoding,
		Type:     kiCERTIFICATE,
	}
}

func NewKICaCertificate(encoding string) *KeyInfo {
	return &KeyInfo{
		DataName: kiCaCertificate,
		Encoding: encoding,
		Type:     kiCACERTIFICATE,
	}
}

func NewKIError(encoding string, err error) *KeyInfo {
	return &KeyInfo{
		Encoding: encoding,
		Error:    err,
	}
}

func (ki *KeyInfo) SetHeaders(headers map[string]string) *KeyInfo {
	ki.Headers = headers
	return ki
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

// MarshalJSON is used by the go json marshaller. Using a custom one here
// allows us a high degree of control over the resulting output. For example,
// it allows us to use the same struct here to encapsulate both keys and
// certificate, and still have somewhat differenciated output
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
