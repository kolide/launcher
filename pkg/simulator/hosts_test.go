package simulator

import (
	"regexp"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadHostsErrors(t *testing.T) {
	testCases := []struct {
		dir      string
		matchErr string
	}{
		{
			"testdata/invalid_dir",
			"listing hosts directory",
		},
		{
			"testdata/bad_symlink",
			"reading file",
		},
		{
			"testdata/invalid_yaml",
			"unmarshal yaml",
		},
		{
			"testdata/duplicate",
			"duplicate host",
		},
		{
			"testdata/missing_parent",
			"missing parent",
		},
		{
			"testdata/invalid_regexp",
			"compile regexp",
		},
		// TODO add test with cycle
	}

	for _, tt := range testCases {
		t.Run(tt.matchErr, func(t *testing.T) {
			hosts, err := LoadHosts(tt.dir, log.NewNopLogger())
			assert.Nil(t, hosts)
			if assert.NotNil(t, err) {
				assert.Contains(t, err.Error(), tt.matchErr)
			}
		})
	}
}

func TestLoadHosts(t *testing.T) {
	hosts, err := LoadHosts("testdata/valid1", log.NewNopLogger())
	require.Nil(t, err)

	foo, bar := hosts["foo"], hosts["bar"]
	require.NotNil(t, foo)
	require.NotNil(t, bar)

	assert.Equal(t, "foo", foo.Name)
	assert.Equal(t,
		[]matcher{
			matcher{
				regexp.MustCompile("select hour, minutes from time"),
				[]map[string]string{{"hour": "19", "minutes": "34"}},
			},
			matcher{
				regexp.MustCompile("select platform from osquery_info"),
				[]map[string]string{{"platform": "darwin"}},
			},
		},
		foo.Queries,
	)
	assert.Nil(t, foo.parent)

	assert.Equal(t, "bar", bar.Name)
	assert.Equal(t,
		[]matcher{
			{
				regexp.MustCompile("select version from osquery_info"),
				[]map[string]string{{"version": "2.10.2"}},
			},
		},
		bar.Queries,
	)
	assert.Equal(t, foo, bar.parent)
}

func TestRunQuery(t *testing.T) {
	h1 := &queryRunner{
		Queries: []matcher{
			{regexp.MustCompile(".*time.*"), []map[string]string{{"foo": "bar"}}},
		},
		unmatchedQueries: make(map[string]bool),
		logger:           log.NewNopLogger(),
	}
	h2 := &queryRunner{
		Queries: []matcher{
			{regexp.MustCompile("select \\* from osquery_info"), []map[string]string{{"osquery": "info"}}},
		},
		parent:           h1,
		unmatchedQueries: make(map[string]bool),
		logger:           log.NewNopLogger(),
	}
	h3 := &queryRunner{
		Queries: []matcher{
			{regexp.MustCompile("select hour from time"), []map[string]string{{"hour": "12"}}},
			{regexp.MustCompile("select .* from time"), []map[string]string{{"minute": "36"}}},
		},
		parent:           h1,
		unmatchedQueries: make(map[string]bool),
		logger:           log.NewNopLogger(),
	}

	testCases := []struct {
		Host   *queryRunner
		Query  string
		Result []map[string]string
	}{
		{
			h1,
			"select * from time",
			[]map[string]string{{"foo": "bar"}},
		},
		{
			h1,
			"select nomatch",
			nil,
		},
		{
			h2,
			"select * from osquery_info",
			[]map[string]string{{"osquery": "info"}},
		},
		{
			h2,
			"select hour from time",
			[]map[string]string{{"foo": "bar"}},
		},
		{
			h2,
			"select nomatch",
			nil,
		},
		{
			h3,
			"select hour from time",
			[]map[string]string{{"hour": "12"}},
		},
		{
			h3,
			"select day from time",
			[]map[string]string{{"minute": "36"}},
		},
		{
			h3,
			"select day from osquery_info join time",
			[]map[string]string{{"foo": "bar"}},
		},
	}

	for _, tt := range testCases {
		t.Run("", func(t *testing.T) {
			res, err := tt.Host.RunQuery(tt.Query)
			if tt.Result != nil {
				assert.Equal(t, tt.Result, res)
			} else {
				assert.NotNil(t, err)
			}
		})
	}
}
