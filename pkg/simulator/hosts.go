package simulator

import (
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/ghodss/yaml"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
)

type queryRunner struct {
	// Name of this host type.
	Name string `json:"name"`
	// parentName is the name of the parent type.
	ParentName string `json:"parent"`
	// parent is a pointer to the parent (nil if no parent) used for query
	// result inheritance.
	parent *queryRunner
	//QueryResults maps from regexp pattern to query results that should be
	//returned.
	Queries []matcher `json:"queries"`

	// The following members facilitate logging unmatched queries.
	logger           log.Logger
	unmatchedMutex   sync.Mutex
	unmatchedQueries map[string]bool
}

// matcher contains a regex for matching input queries, and the results to
// return if the query matches.
type matcher struct {
	// Pattern is a regexp for the query patterns this should match.
	Pattern regexp.Regexp `json:"pattern"`
	// Results is the results to return for matched queries
	Results []map[string]string `json:"results"`
}

// querySpec exists for loading from the YAML files. After it is parsed into
// this structure, a queryRunner is created by compiling the regexes and
// linking the parents.
type querySpec struct {
	Name       string `json:"name"`
	ParentName string `json:"parent"`
	Queries    []struct {
		Pattern string              `json:"pattern"`
		Results []map[string]string `json:"results"`
	} `json:"queries"`
}

// LoadHosts will load the host specifications and return a map of the
// queryRunners representing these host types.
func LoadHosts(dir string, logger log.Logger) (map[string]*queryRunner, error) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, errors.Wrap(err, "listing hosts directory")
	}

	hostMap := map[string]*queryRunner{}

	// Load all files
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".yaml") {
			path := filepath.Join(dir, file.Name())
			contents, err := ioutil.ReadFile(path)
			if err != nil {
				return nil, errors.Wrapf(err, "reading file %s", path)
			}

			var h querySpec
			err = yaml.Unmarshal(contents, &h)
			if err != nil {
				return nil, errors.Wrapf(err, "unmarshal yaml for %s", path)
			}

			runner := &queryRunner{
				Name:             h.Name,
				ParentName:       h.ParentName,
				Queries:          []matcher{},
				unmatchedQueries: make(map[string]bool),
				logger:           logger,
			}

			for _, q := range h.Queries {
				re, err := regexp.Compile(strings.ToLower(q.Pattern))
				if err != nil {
					return nil, errors.Wrapf(err, "compile regexp for %s", path)
				}
				runner.Queries = append(runner.Queries, matcher{*re, q.Results})
			}

			// Check for duplicate host type name. It is user error
			// to provide multiple definitions for the same host
			// type.
			if _, exists := hostMap[runner.Name]; exists {
				return nil, errors.Errorf("duplicate host type: %s", runner.Name)
			}

			hostMap[runner.Name] = runner
		}
	}

	// Link parents
	for _, runner := range hostMap {
		if runner.ParentName == "" {
			continue
		}

		parent, ok := hostMap[runner.ParentName]
		if !ok {
			return nil, errors.Errorf("missing parent named: %s", runner.ParentName)
		}
		runner.parent = parent
	}

	// TODO check for cycles

	return hostMap, nil
}

func (h *queryRunner) RunQuery(sql string) (rows []map[string]string, err error) {
	sql = strings.ToLower(sql)
	defer func() {
		if err == nil {
			// Query was matched
			return
		}

		h.unmatchedMutex.Lock()
		defer h.unmatchedMutex.Unlock()

		if h.unmatchedQueries[sql] {
			// Already logged this one
			return
		}

		h.unmatchedQueries[sql] = true
		level.Info(h.logger).Log(
			"msg", "host has no match for query",
			"host_type", h.Name,
			"sql", sql,
		)
	}()

	return h.runQueryRecurse(sql)
}

func (h *queryRunner) runQueryRecurse(sql string) ([]map[string]string, error) {
	// Try matching patterns
	for _, q := range h.Queries {
		if q.Pattern.MatchString(sql) {
			return q.Results, nil
		}
	}

	// No patterns matched
	if h.parent == nil {
		// No parent exists
		return nil, errors.New("no matching query pattern")
	}

	// Recursive call to inherited patterns of parent
	return h.parent.runQueryRecurse(sql)
}
