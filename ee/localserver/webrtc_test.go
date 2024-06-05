package localserver

import (
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/require"
)

func TestWebrtc(t *testing.T) {
	// Paste from Fiddle https://jsfiddle.net/e41tgovp/
	remoteSessionDescription := ""

	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := multislogger.New(slog.NewJSONHandler(&logBytes, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ls := localServer{
		slogger: slogger.Logger,
	}

	conn, err := ls.newWebrtcHandler(remoteSessionDescription)
	require.NoError(t, err)

	defer conn.close()

	localSessionDescription, err := conn.localDescription()
	require.NoError(t, err)

	fmt.Println(localSessionDescription)

	time.Sleep(1 * time.Minute)

	fmt.Println(logBytes.String())
}
