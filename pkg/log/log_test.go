package log

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractOsqueryCaller(t *testing.T) {
	testCases := []struct {
		log      string
		expected string
	}{
		{
			`I1101 19:21:40.292618 84815872 distributed.cpp:133] Executing distributed query: kolide:populate:practices:1: SELECT COUNT(*) AS result FROM (select * from time);`,
			`distributed.cpp:133`,
		},
		{
			`E1201 08:21:54.254618 84815872 foobar.m:47] Penguin`,
			`foobar.m:47`,
		},
		{
			`E1201 08:21:54.254618 84815872 unknown] Penguin`,
			``,
		},
		{
			`Just plain bad`,
			``,
		},
	}

	for _, tt := range testCases {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, tt.expected, extractOsqueryCaller(tt.log))
		})
	}
}
