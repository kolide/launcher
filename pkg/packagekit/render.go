package packagekit

type InitOptions struct {
	Name        string
	Description string
	Path        string
	Environment map[string]string
	Flags       []string
}
