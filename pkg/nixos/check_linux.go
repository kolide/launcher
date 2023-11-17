package nixos

import "os"

func IsNixOS() bool {
	if _, err := os.Stat("/etc/NIXOS"); err == nil {
		return true
	}

	return false
}
