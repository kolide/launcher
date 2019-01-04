package packagekit

type InitOptions struct {
	Name        string
	Description string
	Identifier  string
	Path        string
	Environment map[string]string `plist:"EnvironmentVariables"`
	Flags       []string          `plist:"ProgramArguments"`
}
