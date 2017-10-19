package debug

import (
	"io/ioutil"
	"net/http"

	"github.com/e-dard/netbug"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/google/uuid"
	"github.com/pkg/errors"
)

// AttachDebugEndpoints will attach the
func StartDebugServer(addr, tokenPath string, logger log.Logger) error {
	// Generate new (random) UUID
	token, err := uuid.NewRandom()
	if err != nil {
		return errors.Wrap(err, "generating debug token")
	}

	if err := ioutil.WriteFile(tokenPath, []byte(token.String()), 0600); err != nil {
		return errors.Wrap(err, "writing debug token")
	}

	r := http.NewServeMux()
	netbug.RegisterAuthHandler(token.String(), "/debug/", r)
	go func() {
		if err := http.ListenAndServe(addr, r); err != nil {
			level.Info(logger).Log("msg", "starting debug server failed", "err", err)
		}
	}()

	return nil
}
