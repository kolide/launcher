package dataflatten

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
)

type Querier struct {
	rows   []Row
	logger log.Logger
}

type QuerierOpt func(*Querier)

func WithLoggerQ(logger log.Logger) QuerierOpt {
	return func(qr *Querier) {
		qr.logger = logger
	}
}

// FIXME: should this return an interface{}
// FIXME: What query dsl? ruby's pattern matching?
//
// We're traversing a nested data structure
// Query should be terms for each layer.
// (Each time we descend, we pop the query stack)
// need to support array index, or array-as-map
// This does _not_ allow for complex boolean operation.
func Query(data interface{}, query []string, opts ...QuerierOpt) (string, error) {
	qr := &Querier{
		logger: log.NewNopLogger(),
	}

	for _, opt := range opts {
		opt(qr)
	}

	result, err := qr.queryDescend(data, query, 0)
	// FIXME: This stringify is meh
	return fmt.Sprintf("%-v", result), err
}

func (qr *Querier) queryDescend(data interface{}, query []string, depth int) (interface{}, error) {
	if len(query) == 0 {
		return data, nil
	}
	// shift off the first element
	q := query[0]
	query = query[1:]

	if q == "" {
		return "", errors.New("empty query")
	}

	switch v := data.(type) {
	case map[string]interface{}:
		if newData, ok := v[q]; ok {
			return qr.queryDescend(newData, query, depth+1)
		}
		return "", errors.New("not found")
	case []interface{}:
		if arrayKeyValue := strings.Split(q, `=>`); len(arrayKeyValue) == 2 {
			keyName := arrayKeyValue[0]
			keyQuery := arrayKeyValue[1]

			for _, element := range v {
				if elementAsMap, ok := element.(map[string]interface{}); ok {
					if keyvalue, ok := elementAsMap[keyName]; ok {
						// FIXME: need stringify right here. Else floating numbers kill us
						if valueAsString, ok := keyvalue.(string); ok && valueAsString == keyQuery {
							return qr.queryDescend(element, query, depth+1)
						}
					}
				}

			}
			return "", errors.Errorf("key not found %s", q)
		}

		if i, err := strconv.Atoi(q); err == nil {
			if len(v) >= i+1 {
				return qr.queryDescend(v[i], query, depth+1)
			}
			return nil, errors.New("index out of range")
		}

		return nil, errors.New("key mismatch")

	default:
		return "", errors.Errorf(`can't query through object "%-v"`, v)
	}

}

func QueryJson(rawdata []byte, query []string) (string, error) {
	var data interface{}

	if err := json.Unmarshal(rawdata, &data); err != nil {
		return "", errors.Wrap(err, "unmarshalling json")
	}

	return Query(data, query)
}
