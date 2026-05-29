//go:build darwin

package nativemessaging

var allowlistedChromePaths = map[string]struct{}{
	`/Applications/Google Chrome.app/Contents/MacOS/Google Chrome`: struct{}{},
}
