package consoleuser

import (
	"context"
	"os/user"
)

func CurrentUsers(ctx context.Context) ([]*user.User, error) {
	currentUids, err := CurrentUids(ctx)
	if err != nil {
		return nil, err
	}

	users := make([]*user.User, len(currentUids))
	for i, uid := range currentUids {
		u, err := user.LookupId(uid)
		if err != nil {
			return nil, err
		}

		users[i] = u
	}

	return users, nil
}
