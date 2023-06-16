package httpsenderlog

import (
	"fmt"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/sendbuffer"
)

const (
	truncatedFormatString = "%s[TRUNCATED]"
)

type httpSenderLog struct {
	logger     log.Logger
	sendBuffer *sendbuffer.SendBuffer
}

func New(endpoint string) *httpSenderLog {
	sender := sendbuffer.NewHttpSender(endpoint)
	sendBuffer := sendbuffer.New(sender)
	sendBuffer.StartSending()

	return &httpSenderLog{
		sendBuffer: sendBuffer,
		logger:     log.NewJSONLogger(sendBuffer),
	}
}

func (h *httpSenderLog) Log(keyvals ...interface{}) error {
	filterResults(keyvals...)
	return h.logger.Log(keyvals...)
}

func (h *httpSenderLog) StartSending() {
	h.sendBuffer.StartSending()
}

func (h *httpSenderLog) StopSending() {
	h.sendBuffer.StopSending()
}

// filterResults filteres out the osquery results,
// which just make a lot of noise in our debug logs.
// It's a bit fragile, since it parses keyvals, but
// hopefully that's good enough
func filterResults(keyvals ...interface{}) {
	// Consider switching on `method` as well?
	for i := 0; i < len(keyvals); i += 2 {
		if keyvals[i] == "results" && len(keyvals) > i+1 {
			str, ok := keyvals[i+1].(string)
			if ok && len(str) > 100 {
				keyvals[i+1] = fmt.Sprintf(truncatedFormatString, str[0:99])
			}
		}
	}
}
