package consoleuser

import (
	"context"
	"os/user"

	"github.com/kolide/launcher/ee/observability"
)

type ConsoleUser struct {
	Uid            string
	UserProcessPid int32 // Only relevant for Windows, where we want explorer.exe's PID to perform access token lookups
}

func CurrentUsers(ctx context.Context) ([]*user.User, error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	currentUids, err := CurrentUids(ctx)
	if err != nil {
		return nil, err
	}

	users := make([]*user.User, len(currentUids))
	for i, uid := range currentUids {
		u, err := user.LookupId(uid.Uid)
		if err != nil {
			return nil, err
		}

		users[i] = u
	}

	return users, nil
}
