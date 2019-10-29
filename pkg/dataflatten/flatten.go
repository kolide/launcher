package dataflatten

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
)

type Flattener struct {
	includeNils     bool
	rows            []Row
	logger          log.Logger
	query           []string
	queryWildcard   string
	queryKeyDenoter string
}

type FlattenOpts func(*Flattener)

func IncludeNulls() FlattenOpts {
	return func(fl *Flattener) {
		fl.includeNils = true
	}
}

func WithLogger(logger log.Logger) FlattenOpts {
	return func(fl *Flattener) {
		fl.logger = logger
	}
}

func WithQuery(q []string) FlattenOpts {
	return func(fl *Flattener) {
		fl.query = q
	}
}

// TODO: Write this better
// Note that this returns an array with an unstable order.
func Flatten(data interface{}, opts ...FlattenOpts) ([]Row, error) {
	fl := &Flattener{
		rows:            []Row{},
		logger:          log.NewNopLogger(),
		queryWildcard:   `*`,
		queryKeyDenoter: `#`,
	}

	for _, opt := range opts {
		opt(fl)
	}

	if err := fl.descend([]string{}, data, 0); err != nil {
		return nil, err
	}

	return fl.rows, nil
}

func (fl *Flattener) descend(path []string, data interface{}, depth int) error {
	queryTerm, isQueryMatched := fl.queryAtDepth(depth)

	logger := log.With(fl.logger,
		"caller", "descend",
		"depth", depth,
		"rows-so-far", len(fl.rows),
		"query", queryTerm,
	)

	//level.Debug(logger).Log("data", spew.Sdump(data))

	switch v := data.(type) {
	case []interface{}:
		for i, e := range v {
			pathKey := strconv.Itoa(i)
			level.Debug(logger).Log("msg", "checking a array", "path", strings.Join(path, "/"), "indexStr", pathKey)

			// If the queryTerm starts with
			// queryKeyDenoter, then we want to rewrite
			// the path based on it. Note that this does
			// no sanity checking. Multiple values will
			// re-write. If the value isn't there, you get
			// nothing. Etc.
			//
			// keyName == "name"
			// keyValue == "alice" (need to test this againsty queryTerm
			// pathKey == What we descend with
			if strings.HasPrefix(queryTerm, fl.queryKeyDenoter) {
				keyQuery := strings.SplitN(strings.TrimPrefix(queryTerm, fl.queryKeyDenoter), "=>", 2)
				keyName := keyQuery[0]
				keyQueryTerm := fl.queryWildcard

				level.Info(logger).Log("msg", "attempting to coerce array into map", "array keyname", keyName)
				fmt.Printf("seph coercing with %s\n", keyName)

				if elementAsMap, ok := e.(map[string]interface{}); ok {
					val, ok := elementAsMap[keyName]
					if !ok {
						fmt.Println("seph key not found")
						continue
					}

					pathKey, ok = val.(string)
					if !ok {
						fmt.Println("seph val not string")
						continue
					}

					if !(isQueryMatched || fl.queryMatchString(pathKey, keyQueryTerm)) {
						fmt.Println("seph val not matched %s", keyQueryTerm)
						level.Debug(logger).Log("msg", "query not matched", "array keyname", keyName)
						continue
					}

				}
			} else {
				if !(isQueryMatched || fl.queryMatchArrayElement(e, i, queryTerm)) {
					level.Debug(logger).Log("msg", "query not matched", "array index", i)
					continue
				}
			}

			if err := fl.descend(append(path, pathKey), e, depth+1); err != nil {
				return errors.Wrap(err, "flattening array")
			}
		}
	case map[string]interface{}:
		level.Debug(logger).Log("msg", "checking a map", "path", strings.Join(path, "/"))

		for k, e := range v {

			// Check that the key name matches. If not, skip this enture
			// branch of the map
			if !(isQueryMatched || fl.queryMatchString(k, queryTerm)) {
				continue
			}

			if err := fl.descend(append(path, k), e, depth+1); err != nil {
				return errors.Wrap(err, "flattening map")
			}
		}
	case nil:
		if !(isQueryMatched || fl.queryMatchNil(queryTerm)) {
			level.Debug(logger).Log("msg", "query not matched")
			return nil
		}
		fl.rows = append(fl.rows, Row{Path: path, Value: ""})
	default:
		// non-iterable. stringify and be done
		stringValue, err := stringify(v)
		if err != nil {
			return errors.Wrapf(err, "flattening at path %v", path)
		}

		if !(isQueryMatched || fl.queryMatchString(stringValue, queryTerm)) {
			level.Debug(logger).Log("msg", "query not matched")
			return nil
		}
		fl.rows = append(fl.rows, Row{Path: path, Value: stringValue})

	}
	return nil

}

func (fl *Flattener) queryMatchNil(queryTerm string) bool {
	// FIXME, we should do something with queryTerm
	return fl.includeNils
}

// queryMatchArray matches arrays. This one is magic.
//
// Syntax:
//   #i -- Match index i. For example `#0`
//   k=>queryTerm -- If this is a map, it should have key k, that matches queryTerm
//
// We use `=>` as something that is reasonable intutive, and no very
// likely to occur on it's own. Unfortunately, `==` shows up in base64
func (fl *Flattener) queryMatchArrayElement(data interface{}, arrIndex int, queryTerm string) bool {
	// strip off the key re-write denotation before trying to match
	queryTerm = strings.TrimPrefix(queryTerm, fl.queryKeyDenoter)

	if queryTerm == fl.queryWildcard {
		return true
	}

	// If the queryTerm is an int, then we expect to match the index
	if queryIndex, err := strconv.Atoi(queryTerm); err == nil {
		return queryIndex == arrIndex
	}

	switch dataCasted := data.(type) {
	case []interface{}:
		// fails. We can't match an array that has arrays as elements. Use a wildcard
		return false
	case map[string]interface{}:
		kvQuery := strings.SplitN(queryTerm, "=>", 2)
		for k, v := range dataCasted {
			// Since this needs to check against _every_
			// member, return true. Or fall through to the
			// false.
			if fl.queryMatchString(k, kvQuery[0]) && fl.queryMatchStringify(v, kvQuery[1]) {
				return true
			}
		}
		return false
	default:
		// non-iterable. stringify and be done
		return fl.queryMatchStringify(dataCasted, queryTerm)
	}

	// Else, we need to think harder
	return false
}

func (fl *Flattener) queryMatchStringify(data interface{}, queryTerm string) bool {
	// strip off the key re-write denotation before trying to match
	queryTerm = strings.TrimPrefix(queryTerm, fl.queryKeyDenoter)

	if queryTerm == fl.queryWildcard {
		return true
	}

	if data == nil {
		return fl.queryMatchNil(queryTerm)
	}

	stringValue, err := stringify(data)
	if err != nil {
		return false
	}

	return fl.queryMatchString(stringValue, queryTerm)

}

func (fl *Flattener) queryMatchString(v, queryTerm string) bool {
	if queryTerm == fl.queryWildcard {
		return true
	}

	// Some basic string manipulations to handle prefix and suffix operations
	switch {
	case strings.HasPrefix(queryTerm, fl.queryWildcard) && strings.HasSuffix(queryTerm, fl.queryWildcard):
		queryTerm = strings.TrimPrefix(queryTerm, fl.queryWildcard)
		queryTerm = strings.TrimSuffix(queryTerm, fl.queryWildcard)
		return strings.Contains(v, queryTerm)

	case strings.HasPrefix(queryTerm, fl.queryWildcard):
		queryTerm = strings.TrimPrefix(queryTerm, fl.queryWildcard)
		return strings.HasSuffix(v, queryTerm)

	case strings.HasSuffix(queryTerm, fl.queryWildcard):
		queryTerm = strings.TrimSuffix(queryTerm, fl.queryWildcard)
		return strings.HasPrefix(v, queryTerm)
	}

	return v == queryTerm
}

func (fl *Flattener) queryAtDepth(depth int) (string, bool) {
	// if we're nil, there's an implied wildcard
	//
	// This works because:
	// []string   is len 0, and nil
	// []string{} is len 0, but not nil
	if fl.query == nil {
		return fl.queryWildcard, true
	}

	// If there's no query for this depth, then there's an implied
	// wildcard. This allows the query to specify prefixes.
	if depth+1 > len(fl.query) {
		return fl.queryWildcard, true
	}

	q := fl.query[depth]

	return q, q == fl.queryWildcard
}

// extractKeyNameFromMap will return the value from a map, if it has
// an appropriately named key, whose value can be stringified
// FIXME: This function should probably be remoed
func extractKeyNameFromMap(data map[string]interface{}, keyname string, deleteKey bool) string {
	if val, ok := data[keyname]; ok {
		if vString, err := stringify(val); err == nil {
			if deleteKey {
				delete(data, keyname)
			}
			return vString
		}
	}
	return ""
}

// stringify takes an arbitrary piece of data, and attempst to coerce
// it into a string.
func stringify(data interface{}) (string, error) {
	switch v := data.(type) {
	case nil:
		return "", nil
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	case uint64:
		return strconv.FormatUint(v, 10), nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case int:
		return strconv.Itoa(v), nil
	case bool:
		return strconv.FormatBool(v), nil
	case time.Time:
		return strconv.FormatInt(v.Unix(), 10), nil
	default:
		//spew.Dump(data)
		return "", errors.Errorf("unknown type on %v", data)
	}
}
