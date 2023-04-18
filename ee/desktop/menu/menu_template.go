package menu

import (
	"fmt"
	"strings"
	"text/template"
	"time"
)

const (
	funcHasCapability = "hasCapability"
	funcRelativeTime  = "relativeTime"
)

type TemplateData struct {
	LauncherVersion    string `json:",omitempty"`
	LauncherRevision   string `json:",omitempty"`
	GoVersion          string `json:",omitempty"`
	ServerHostname     string `json:",omitempty"`
	LastMenuUpdateTime int64  `json:",omitempty"`
}

type templateParser struct {
	td *TemplateData
}

func NewTemplateParser(td *TemplateData) *templateParser {
	tp := &templateParser{
		td: td,
	}

	return tp
}

// Parse parses text as a template body for the menu template data
// if an error occurs while parsing, an empty string is returned along with the error
func (tp *templateParser) Parse(text string) (string, error) {
	if tp == nil || tp.td == nil {
		return "", fmt.Errorf("templateData is nil")
	}

	t, err := template.New("menu_template").Funcs(template.FuncMap{
		// hasCapability enables interoperability between different versions of launcher
		funcHasCapability: func(capability string) bool {
			if capability == funcRelativeTime {
				return true
			}
			return false
		},
		// relativeTime takes a Unix timestamp and returns a fuzzy timestamp
		funcRelativeTime: func(timestamp int64) string {
			currentTime := time.Now().Unix()
			diff := timestamp - currentTime

			switch {
			case diff < -60*60: // more than an hour ago
				return fmt.Sprintf("%d Hours Ago", -diff/3600)
			case diff < -60*2: // more than 2 minutes ago
				return fmt.Sprintf("%d Minutes Ago", -diff/60)
			case diff < -90: // more than 90 seconds ago
				return fmt.Sprintf("%d Seconds Ago", -diff)
			case diff < -50: // more than 50 seconds ago
				return "One Minute Ago"
			case diff < -5: // more than 5 seconds ago
				return fmt.Sprintf("%d Seconds Ago", -diff)
			case diff < 0: // in the last 5 seconds
				return "Just Now"
			case diff < 60*10: // less than 10 minutes
				return "Very Soon"
			case diff < 60*50: // less than 50 minutes
				return fmt.Sprintf("In %d Minutes", diff/60)
			case diff < 60*90: // less than 90 minutes
				return "In About An Hour"
			case diff < 60*60*2: // less than 2 hours
				return fmt.Sprintf("In %d Minutes", diff/60)
			case diff < 60*60*23: // less than 23 hours
				return fmt.Sprintf("In %d Hours", diff/3600)
			case diff < 60*60*36: // less than 36 hours
				return "In One Day"
			case diff < 60*60*24*14: // less than 14 days
				return fmt.Sprintf("In %d Days", diff/86400)
			default: // 2 weeks or more
				return fmt.Sprintf("In %d Weeks", diff/604800)
			}
		},
	}).Parse(text)
	if err != nil {
		return "", fmt.Errorf("could not parse template: %w", err)
	}

	var b strings.Builder
	if err := t.Execute(&b, tp.td); err != nil {
		return "", fmt.Errorf("could not write template output: %w", err)
	}

	return b.String(), nil
}
