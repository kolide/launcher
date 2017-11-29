package simulator

import (
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ghodss/yaml"

	"github.com/pkg/errors"
)

type Host struct {
	// Name of this host type.
	Name string `json:"name"`
	// parentName is the name of the parent type.
	ParentName string `json:"parent"`
	// parent is a pointer to the parent (nil if no parent) used for query
	// result inheritance.
	parent *Host
	//QueryResults maps from regexp pattern to query results that should be
	//returned.
	Queries []matcher `json:"queries"`
}

type matcher struct {
	// Pattern is a regexp for the query patterns this should match.
	Pattern regexp.Regexp `json:"pattern"`
	// Results is the results to return for matched queries
	Results []map[string]string `json:"results"`
}

type hostYAML struct {
	Name       string `json:"name"`
	ParentName string `json:"parent"`
	Queries    []struct {
		Pattern string              `json:"pattern"`
		Results []map[string]string `json"results"`
	} `json:"queries"`
}

func LoadHosts(dir string) (map[string]*Host, error) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, errors.Wrap(err, "listing hosts directory")
	}

	hostMap := map[string]*Host{}

	// Load all files
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".yaml") {
			path := filepath.Join(dir, file.Name())
			contents, err := ioutil.ReadFile(path)
			if err != nil {
				return nil, errors.Wrapf(err, "reading file %s", path)
			}

			var h hostYAML
			err = yaml.Unmarshal(contents, &h)
			if err != nil {
				return nil, errors.Wrapf(err, "unmarshal yaml for %s", path)
			}

			host := Host{
				Name:       h.Name,
				ParentName: h.ParentName,
				Queries:    []matcher{},
			}

			for _, q := range h.Queries {
				re, err := regexp.Compile(q.Pattern)
				if err != nil {
					return nil, errors.Wrapf(err, "compile regexp for %s", path)
				}
				host.Queries = append(host.Queries, matcher{*re, q.Results})
			}

			// Check for duplicate host type name
			if _, exists := hostMap[host.Name]; exists {
				return nil, errors.Errorf("duplicate host type: %s", host.Name)
			}

			hostMap[host.Name] = &host
		}
	}

	// Link parents
	for _, host := range hostMap {
		if host.ParentName == "" {
			continue
		}

		parent, ok := hostMap[host.ParentName]
		if !ok {
			return nil, errors.Errorf("missing parent named: %s", host.ParentName)
		}
		host.parent = parent
	}

	// TODO check for cycles

	return hostMap, nil
}

func (h *Host) RunQuery(sql string) ([]map[string]string, error) {
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
	return h.parent.RunQuery(sql)
}
