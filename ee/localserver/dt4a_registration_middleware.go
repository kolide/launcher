package localserver

import (
	"log/slog"
	"net/http"

	"github.com/kolide/launcher/pkg/traces"
)

func (ls *localServer) dt4aRegistrationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r, span := traces.StartHttpRequestSpan(r)
		defer span.End()

		slogger := ls.slogger.With("component", "dt4a_registration_middleware")

		accountUuid, userUuid := r.Header.Get(dt4aAccountUuidHeaderKey), r.Header.Get(dt4aUserUuidHeaderKey)

		if accountUuid == "" || userUuid == "" {
			slogger.Log(r.Context(), slog.LevelWarn,
				"missing dt4a account or user info",
			)

			http.Error(w, "missing dt4a account or user info", http.StatusBadRequest)
			return
		}

		slogger.Log(r.Context(), slog.LevelDebug,
			"found dt4a account and user info",
		)

		// TODO / figure out:
		/*
			- if agent registered and matches info, continue
			- if agent registered but does not match info, return error
			- if agent not registered, register agent
			- ... TBD
		*/

		next.ServeHTTP(w, r)
	})
}
