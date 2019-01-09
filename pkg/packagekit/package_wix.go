package packagekit

import "github.com/kolide/launcher/pkg/packagekit/internal"

func GimmieWXS() ([]byte, error) {
	return internal.InstallWXS()
}
