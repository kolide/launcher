package dev_table_tooling

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/stretchr/testify/assert"
)

func Test_generate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		commandName    []string
		expectedResult []map[string]string
	}{
		{
			name: "no command name",
		},
		{
			name:        "malware",
			commandName: []string{"ransomware.exe"},
		},
		{
			name:           "should-always-work happy path",
			commandName:    []string{"echo"},
			expectedResult: []map[string]string{{"name": "echo", "args": "hello", "output": base64.StdEncoding.EncodeToString([]byte("hello\n"))}},
		},
	}

	table := Table{logger: log.NewNopLogger()}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			constraints := make(map[string][]string)
			constraints["name"] = tt.commandName

			got, _ := table.generate(context.Background(), tablehelpers.MockQueryContext(constraints))

			assert.ElementsMatch(t, tt.expectedResult, got)
		})
	}
}
