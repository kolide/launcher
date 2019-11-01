// Package dataflatten contains tools to flatten complex data
// structures.
//
// On macOS, many plists use an array of maps, these can be tricky to
// filter. This package knows how to flatten that structure, as well
// as rewriting it as a nested array, or filtering it. It is akin to
// xpath, though simpler.
//
// This tool works primarily through string interfaces, so type
// information may be lost.
//
// Query Syntax
//
// The query syntax handles both filtering and basic rewriting. It is
// not perfect. The idea behind it, is that we descend through an data
// structure, specifying what matches at each level.
//
// Each level of query can do:
//  * specify a filter, this is a simple string match with wildcard support. (prefix and/or postfix, but not infix)
//  * If the data is an array, specify an index
//  * For array-of-maps, specify a key to rewrite as a nested map
//
// Each query term has 3 parts: [#]string[=>kvmatch]
//   1. An optional `#` This denotes a key to rewrite an array-of-maps with
//   2. A search term. If this is an integer, it is interpreted as an array index.
//   3. a key/value match string. For a map, this is to match the value of a key.
//
//  Some examples:
//  *  data/users            Return everything under { data: { users: { ... } } }
//  *  data/users/0          Return the first item in the users array
//  *  data/users/name=>A*   Return users whose name starts with "A"
//  *  data/users/#id        Return the users, and rewrite the users array to be a map with the id as the key
//
// See the test suite for extensive examples.
package dataflatten

import (
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
)

// Flattener is an interface to flatten complex, nested, data
// structures. It recurses through them, and returns a simplified
// form. At the simplest level, this rewrites:
//
//   { foo: { bar: { baz: 1 } } }
//
// To:
//
//   [ { path: foo/bar/baz, value: 1 } ]
//
// It can optionally filtering and rewriting.
type Flattener struct {
	includeNils     bool
	rows            []Row
	logger          log.Logger
	query           []string
	queryWildcard   string
	queryKeyDenoter string
}

type FlattenOpts func(*Flattener)

// IncludeNulls indicates that Flatten should return null values,
// instead of skipping over them.
func IncludeNulls() FlattenOpts {
	return func(fl *Flattener) {
		fl.includeNils = true
	}
}

// WithLogger sets the logger to use
func WithLogger(logger log.Logger) FlattenOpts {
	return func(fl *Flattener) {
		fl.logger = logger
	}
}

// WithQuery Specifies a query to flatten with. This is used both for
// re-writing arrays into maps, and for filtering. See "Query
// Specification" for docs.
func WithQuery(q []string) FlattenOpts {
	return func(fl *Flattener) {
		fl.query = q
	}
}

// Flatten is the entry point to the Flattener functionality.
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

// descend recurses through a given data structure flattening along the way.
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
	// TODO: If needed, we could use queryTerm for optional nil filtering
	return fl.includeNils
}

// queryMatchArrayElement matches arrays. This one is magic.
//
// Syntax:
//   #i -- Match index i. For example `#0`
//   k=>queryTerm -- If this is a map, it should have key k, that matches queryTerm
//
// We use `=>` as something that is reasonably intuitive, and not very
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
