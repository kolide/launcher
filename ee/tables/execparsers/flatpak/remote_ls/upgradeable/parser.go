package flatpak_upgradeable

import (
	"bufio"
	"io"
	"regexp"
)

// The app id conforms to: [dbus bus name specification](https://dbus.freedesktop.org/doc/dbus-specification.html#message-protocol-names)
var flatpakAppIdRegexp = regexp.MustCompile(`((?:[a-zA-Z_-]+[a-zA-Z0-9_-]*\.){1,}[a-zA-Z_-]+[a-zA-Z0-9_-]*)`)

// Wow this has been a head-pounding bugger of a thing.
//
// flatpak remote-ls --updates
// data is separated by whitespace only
// data headers aren't in stdout, they are written directly to TTY
// data column availability differs between linux distributions
// data columns can only be specified on certain linux distributions with `--columns=`
// columns are spaced arbitrarily to fit the window, both in flatpak *pretty* mode and simple mode. `export FLATPAK_FANCY_OUTPUT=0`
// data will be ellipsed if it overflows the column's arbitrary width
// // this happens to a higher degree if the command is ran with `script -c <cmd>` in an attempt to parse the printed headers (the column widths are based on window size from ioctl_tty)
// Take a look at this beauty if you want to: [flatpak-table-printer.c](https://github.com/flatpak/flatpak/blob/main/app/flatpak-table-printer.c)
//
// After traveling various paths of acquiring and preserving as much data as I could, I give up. You win flatpak.
// Thus is why I've resorted to good ole regexp, and only to get the app id.
// The app id is the only* consistent data output between linux distributions, and is used in the flatpak update command.
func remotelsParse(reader io.Reader) (any, error) {
	results := make([]map[string]string, 0)

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()

		id := flatpakAppIdRegexp.FindString(line)
		if id == "" {
			continue
		}

		results = append(results, map[string]string{"id": id})
	}

	return results, nil
}
