package debuglogger

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/kolide/kit/stringutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	reallyLongString    = stringutil.RandomString(200)
	truncatedLongString = fmt.Sprintf(truncatedFormatString, reallyLongString[0:99])
)

func TestFilterResults(t *testing.T) {
	t.Parallel()

	data := []interface{}{
		"one", "two",
		"results", reallyLongString,
	}

	filterResults(data...)
	assert.Len(t, data, 4)
	assert.Equal(t, data[0], "one")
	assert.Equal(t, data[1], "two")
	assert.Equal(t, data[2], "results")
	assert.Len(t, data[3], 110)
	assert.Contains(t, data[3], "[TRUNCATED]")
	assert.Equal(t, data[3], truncatedLongString)
}

func TestKitLogging(t *testing.T) {
	t.Parallel()

	data := []interface{}{
		"one", "two",
		"results", reallyLongString,
	}

	expected := map[string]string{
		"one":     "two",
		"results": truncatedLongString,
	}
	//	expectedJson, err := json.Marshal(expected)
	//require.NoError(t, err, "json marshal expected")

	tmpfile, err := ioutil.TempFile("", "test-debuglogger")
	require.NoError(t, err, "make temp file")
	defer os.Remove(tmpfile.Name())

	// we only need a file path, not the file handle
	tmpfile.Close()

	logger := NewKitLogger(tmpfile.Name())

	logger.Log(data...)

	contentsRaw, err := os.ReadFile(tmpfile.Name())
	require.NoError(t, err, "read temp file")

	var contents map[string]string
	require.NoError(t, json.Unmarshal(contentsRaw, &contents), "unmarshal json")

	// can't compare the whole thing, since we have extra values from timestamp and caller
	for k, v := range expected {
		assert.Equal(t, v, contents[k])
	}

}
