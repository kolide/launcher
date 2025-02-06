package table

import (
	"context"
	"testing"
	"time"

	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestChromeLoginDataEmails(t *testing.T) {
	t.Parallel()

	mockFlags := typesmocks.NewFlags(t)
	mockFlags.On("TableGenerateTimeout").Return(1 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()
	slogger := multislogger.NewNopLogger()

	chromeLoginDataEmailsTable := ChromeLoginDataEmails(mockFlags, slogger)

	require.Equal(t, "kolide_chrome_login_data_emails", chromeLoginDataEmailsTable.Name())

	response := chromeLoginDataEmailsTable.Call(context.TODO(), map[string]string{
		"action":  "generate",
		"context": "{}",
	})

	require.Equal(t, int32(0), response.Status.Code, response.Status.Message) // success
}
