//go:build !darwin

package checkups

import (
	"context"
)

func (q *quarantine) quarantinedAppBundles(_ context.Context) ([]string, error) {
	return nil, nil
}
