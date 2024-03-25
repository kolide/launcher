package data_table

import (
	"bytes"
	_ "embed"
	"testing"

	"github.com/stretchr/testify/require"
)

//go:embed test-data/test.csv
var csv []byte

//go:embed test-data/top.txt
var top []byte

//go:embed test-data/snap.txt
var snap []byte

func TestParse(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		name      string
		input     []byte
		skipLines uint
		headers   []string
		delimiter string
		expected  []map[string]string
	}{
		{
			name:     "empty input",
			expected: make([]map[string]string, 0),
		},
		{
			name:  "snap",
			input: snap,
			expected: []map[string]string{
				{
					"Name":      "core22",
					"Version":   "20240111",
					"Rev":       "1122",
					"Size":      "77MB",
					"Publisher": "canonical**",
					"Notes":     "base",
				},
			},
		},
		{
			name:      "csv",
			input:     csv,
			headers:   []string{"name", "age", "date", "street", "city", "state", "zip"},
			delimiter: ",",
			expected: []map[string]string{
				{
					"name":   "Sara Walton",
					"age":    "19",
					"date":   "07/10/2010",
					"street": "Tagka Manor",
					"city":   "Kedevwir",
					"state":  "WV",
					"zip":    "40036",
				},
				{
					"name":   "Martin Powers",
					"age":    "23",
					"date":   "06/23/1942",
					"street": "Eror Parkway",
					"city":   "Masuzose",
					"state":  "ID",
					"zip":    "92375",
				},
				{
					"name":   "Sara Porter",
					"age":    "53",
					"date":   "01/12/1942",
					"street": "Ipsuj Path",
					"city":   "Kikvitud",
					"state":  "GA",
					"zip":    "26070",
				},
				{
					"name":   "Jayden Riley",
					"age":    "41",
					"date":   "11/30/2008",
					"street": "Rahef Point",
					"city":   "Sirunu",
					"state":  "UT",
					"zip":    "21076",
				},
				{
					"name":   "Genevieve Greene",
					"age":    "58",
					"date":   "04/07/1976",
					"street": "Camguf Terrace",
					"city":   "Cunule",
					"state":  "KS",
					"zip":    "40733",
				},
			},
		},
		{
			name:      "top",
			input:     top,
			skipLines: 11,
			expected: []map[string]string{
				{
					"PID":  "3210",
					"#TH":  "29",
					"MEM":  "2552M",
					"PGRP": "3210",
					"PPID": "1",
					"UID":  "501",
				},
				{
					"PID":  "4933",
					"#TH":  "19/1",
					"MEM":  "1266M",
					"PGRP": "4930",
					"PPID": "4930",
					"UID":  "501",
				},
				{
					"PID":  "400",
					"#TH":  "20",
					"MEM":  "1021M",
					"PGRP": "400",
					"PPID": "1",
					"UID":  "88",
				},
				{
					"PID":  "67777",
					"#TH":  "5",
					"MEM":  "824M",
					"PGRP": "4930",
					"PPID": "67536",
					"UID":  "501",
				},
				{
					"PID":  "1265",
					"#TH":  "26",
					"MEM":  "631M",
					"PGRP": "1258",
					"PPID": "1258",
					"UID":  "501",
				},
				{
					"PID":  "87436",
					"#TH":  "25",
					"MEM":  "511M",
					"PGRP": "84083",
					"PPID": "84083",
					"UID":  "501",
				},
				{
					"PID":  "67534",
					"#TH":  "21",
					"MEM":  "420M",
					"PGRP": "4930",
					"PPID": "4930",
					"UID":  "501",
				},
				{
					"PID":  "3189",
					"#TH":  "37",
					"MEM":  "403M",
					"PGRP": "3189",
					"PPID": "1",
					"UID":  "501",
				},
				{
					"PID":  "579",
					"#TH":  "23",
					"MEM":  "352M",
					"PGRP": "579",
					"PPID": "1",
					"UID":  "0",
				},
				{
					"PID":  "4936",
					"#TH":  "22",
					"MEM":  "312M",
					"PGRP": "4930",
					"PPID": "4930",
					"UID":  "501",
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := NewParser(WithSkipLines(tt.skipLines), WithHeaders(tt.headers), WithDelimiter(tt.delimiter))
			result, err := p.Parse(bytes.NewReader(tt.input))

			require.NoError(t, err, "unexpected error parsing input")
			require.ElementsMatch(t, tt.expected, result)
		})
	}
}
