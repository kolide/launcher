package osquery

// OsqueryDistributedQueryResults represents the format of the results of an
// osquery distributed query
type OsqueryDistributedQueryResults map[string][]map[string]string

// QueryContent is the format of a query stanza in an osquery configuration
type QueryContent struct {
	Query       string  `json:"query"`
	Description string  `json:"description,omitempty"`
	Interval    uint    `json:"interval"`
	Platform    *string `json:"platform,omitempty"`
	Version     *string `json:"version,omitempty"`
	Snapshot    *bool   `json:"snapshot,omitempty"`
	Removed     *bool   `json:"removed,omitempty"`
	Shard       *uint   `json:"shard,omitempty"`
}

// Queries is a helper which represents the format of a set of queries in a pack
type Queries map[string]QueryContent

// PackContent is the format of an osquery query pack
type PackContent struct {
	Platform  string   `json:"platform,omitempty"`
	Version   string   `json:"version,omitempty"`
	Shard     uint     `json:"shard,omitempty"`
	Discovery []string `json:"discovery,omitempty"`
	Queries   Queries  `json:"queries"`
}

// Packs is a helper which represents the format of a list of osquery query packs
type Packs map[string]PackContent

// Decorators is the format of the decorator configuration in an osquery config
type Decorators struct {
	Load     []string            `json:"load,omitempty"`
	Always   []string            `json:"always,omitempty"`
	Interval map[string][]string `json:"interval,omitempty"`
}

// OsqueryConfig is a struct that can be serialized into a valid osquery config
// using Go's JSON tooling
type OsqueryConfig struct {
	Options    map[string]interface{} `json:"options"`
	Decorators Decorators             `json:"decorators,omitempty"`
	Packs      Packs                  `json:"packs,omitempty"`
}

// OsqueryResultLog is the format of an osquery result log (ie: a differential
// or snapshot query)
type OsqueryResultLog struct {
	Name           string `json:"name"`
	HostIdentifier string `json:"hostIdentifier"`
	UnixTime       string `json:"unixTime"`
	CalendarTime   string `json:"calendarTime"`
	// Columns stores the columns of differential queries
	Columns map[string]string `json:"columns,omitempty"`
	// Snapshot stores the rows and columns of snapshot queries
	Snapshot    []map[string]string `json:"snapshot,omitempty"`
	Action      string              `json:"action"`
	Decorations map[string]string   `json:"decorations"`
}

// OsqueryStatusLog is the format of an osquery status log
type OsqueryStatusLog struct {
	Severity    string            `json:"severity"`
	Filename    string            `json:"filename"`
	Line        string            `json:"line"`
	Message     string            `json:"message"`
	Version     string            `json:"version"`
	Decorations map[string]string `json:"decorations"`
}
