package dataflatten

import (
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
		"path", strings.Join(path, "/"),
	)

	switch v := data.(type) {
	case []interface{}:
		for i, e := range v {
			pathKey := strconv.Itoa(i)
			level.Debug(logger).Log("msg", "checking an array", "indexStr", pathKey)

			// If the queryTerm starts with
			// queryKeyDenoter, then we want to rewrite
			// the path based on it. Note that this does
			// no sanity checking. Multiple values will
			// re-write. If the value isn't there, you get
			// nothing. Etc.
			//
			// keyName == "name"
			// keyValue == "alex" (need to test this againsty queryTerm
			// pathKey == What we descend with
			if strings.HasPrefix(queryTerm, fl.queryKeyDenoter) {
				keyQuery := strings.SplitN(strings.TrimPrefix(queryTerm, fl.queryKeyDenoter), "=>", 2)
				keyName := keyQuery[0]

				innerlogger := log.With(logger, "arraykeyname", keyName)
				level.Debug(logger).Log("msg", "attempting to coerce array into map")

				e, ok := e.(map[string]interface{})
				if !ok {
					level.Debug(innerlogger).Log("msg", "can't coerce into map")
					continue
				}

				// Is keyName in this array?
				val, ok := e[keyName]
				if !ok {
					level.Debug(innerlogger).Log("msg", "keyName not in map")
					continue
				}

				pathKey, ok = val.(string)
				if !ok {
					level.Debug(innerlogger).Log("msg", "can't coerce pathKey val into string")
					continue
				}

				// Looks good to descend. we're overwritten both e and pathKey. Exit this conditional.
			}

			if !(isQueryMatched || fl.queryMatchArrayElement(e, i, queryTerm)) {
				level.Debug(logger).Log("msg", "query not matched")
				continue
			}

			if err := fl.descend(append(path, pathKey), e, depth+1); err != nil {
				return errors.Wrap(err, "flattening array")
			}

		}
	case map[string]interface{}:
		level.Debug(logger).Log("msg", "checking a map", "path", strings.Join(path, "/"))
		for k, e := range v {

			// Check that the key name matches. If not, skip this entire
			// branch of the map
			if !(isQueryMatched || fl.queryMatchString(k, queryTerm)) {
				continue
			}

			if err := fl.descend(append(path, k), e, depth+1); err != nil {
				return errors.Wrap(err, "flattening map")
			}
		}
	case nil:
		// Because we want to filter nils out, we do _not_ examine isQueryMatched here
		if !(fl.queryMatchNil(queryTerm)) {
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
	logger := log.With(fl.logger,
		"caller", "queryMatchArrayElement",
		"rows-so-far", len(fl.rows),
		"query", queryTerm,
		"arrIndex", arrIndex,
	)

	// strip off the key re-write denotation before trying to match
	queryTerm = strings.TrimPrefix(queryTerm, fl.queryKeyDenoter)

	if queryTerm == fl.queryWildcard {
		return true
	}

	// If the queryTerm is an int, then we expect to match the index
	if queryIndex, err := strconv.Atoi(queryTerm); err == nil {
		level.Debug(logger).Log("msg", "using numeric index comparison")
		return queryIndex == arrIndex
	}

	level.Debug(logger).Log("msg", "checking data type")

	switch dataCasted := data.(type) {
	case []interface{}:
		// fails. We can't match an array that has arrays as elements. Use a wildcard
		return false
	case map[string]interface{}:
		kvQuery := strings.SplitN(queryTerm, "=>", 2)

		// If this is one long, then we're testing for whether or not there's a key with this name,
		if len(kvQuery) == 1 {
			_, ok := dataCasted[kvQuery[0]]
			return ok
		}

		// Else see if the value matches
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

// queryAtDepth returns the query parameter for a given depth, and
// boolean indicating we've run out of queries. If we've run out of
// queries, than we can start checking, everything is a match.
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
