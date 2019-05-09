// +build !windows

package authenticode

import "context"

func Sign(ctx context.Context, file string, opts ...SigntoolOpt) error {
	return nil
}
