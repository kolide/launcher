package applenotarization

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckSuccess(t *testing.T) {
	t.Parallel()

	ctx := context.TODO()

	var tests = []struct {
		fakeFile       string
		uuid           string
		expectedError  bool
		expectedStatus string
	}{
		{
			fakeFile:       "testdata/info.xml",
			uuid:           "11111111-2222-3333-4444-f4b2a99e443a",
			expectedStatus: "success",
		},
		{
			fakeFile:      "testdata/info.xml",
			uuid:          "mismatched uuid",
			expectedError: true,
		},
		{
			fakeFile:       "testdata/infoinprogress.xml",
			uuid:           "77777777-1111-4444-aaaa-111111111111",
			expectedStatus: "in progress",
		},
	}

	for _, tt := range tests {
		fileBytes, err := ioutil.ReadFile(tt.fakeFile)
		require.NoError(t, err)
		n := New("myname@example.com", "123password", "X11111AAAA")
		n.fakeResponse = string(fileBytes)

		returnedStatus, err := n.Check(ctx, tt.uuid)

		if tt.expectedError {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			require.Equal(t, tt.expectedStatus, returnedStatus)
		}
	}
}

func TestSubmit(t *testing.T) {
	t.Parallel()
	ctx := context.TODO()

	tmpZipFile, err := ioutil.TempFile("", "fake-for-submission.*.zip")
	require.NoError(t, err)
	defer os.Remove(tmpZipFile.Name())

	var tests = []struct {
		fakeFile     string
		expectedUuid string
	}{
		{
			fakeFile:     "testdata/submit.xml",
			expectedUuid: "11111111-aaaa-4444-aaaa-bbbbbbbbbbbb",
		},
		{
			fakeFile:     "testdata/submitduplicate.xml",
			expectedUuid: "22222222-dddd-4444-4444-cccccccccccc",
		},
	}

	for _, tt := range tests {
		fileBytes, err := ioutil.ReadFile(tt.fakeFile)
		require.NoError(t, err)
		n := New("myname@example.com", "123password", "X11111AAAA")
		n.fakeResponse = string(fileBytes)

		returnedUuid, err := n.Submit(ctx, tmpZipFile.Name(), "com.example.testing")
		require.NoError(t, err)
		require.Equal(t, tt.expectedUuid, returnedUuid, "Using fake data in %s", tt.fakeFile)
	}

}
