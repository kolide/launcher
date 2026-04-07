package menu

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/kolide/launcher/v2/ee/agent/types"
)

const (
	CurrentMenuVersion string = "0.1.0" // Bump menu version when major changes occur to the TemplateData format

	// Capabilities queriable via hasCapability
	funcHasCapability     = "hasCapability"
	funcRelativeTime      = "relativeTime"
	errorlessTemplateVars = "errorlessTemplateVars" // capability to evaluate undefined template vars without failing
	errorlessActions      = "errorlessActions"      // capability to evaluate undefined menu item actions without failing
	circleDot             = "circleDot"             // capability to use circle-dot icon

	// TemplateData keys
	LauncherVersion    string = "LauncherVersion"
	LauncherRevision   string = "LauncherRevision"
	GoVersion          string = "GoVersion"
	ServerHostname     string = "ServerHostname"
	LastMenuUpdateTime string = "LastMenuUpdateTime"
	MenuVersion        string = "MenuVersion"
)

type TemplateData map[string]any

type templateParser struct {
	td      *TemplateData
	locData types.LocalizationData
}

func NewTemplateParser(td *TemplateData, locData types.LocalizationData) *templateParser {
	return &templateParser{
		td:      td,
		locData: locData,
	}
}

// pluralize selects the singular or plural form based on count.
func pluralize(singular, plural string, count int64) string {
	if count == 1 {
		return singular
	}
	return plural
}

// localizedDistanceInWords returns a localized distance expression (e.g. "2 horas"),
// or an empty string if translations are unavailable for the configured locale.
func (tp *templateParser) localizedDistanceInWords(singular, plural string, count int64) string {
	tmpl := pluralize(singular, plural, count)
	if tmpl == "" {
		return ""
	}
	return strings.ReplaceAll(tmpl, "%{count}", strconv.FormatInt(count, 10))
}

// wrapRelative wraps a distance expression with the locale's past/future pattern
// (e.g. "hace %{time}" -> "hace 2 horas"). Returns the expression unchanged
// if no relative pattern is available.
func (tp *templateParser) wrapRelative(timeExpr string, isFuture bool) string {
	t, ok := tp.locData.Translations[tp.locData.Locale]
	if !ok {
		return timeExpr
	}

	pattern := t.Datetime.Relative.Past
	if isFuture {
		pattern = t.Datetime.Relative.Future
	}

	if pattern == "" {
		return timeExpr
	}

	return strings.ReplaceAll(pattern, "%{time}", timeExpr)
}

// hasLocalizationData returns true if the parser has a valid locale with translations.
func (tp *templateParser) hasLocalizationData() bool {
	if tp.locData.Locale == "" || len(tp.locData.Translations) == 0 {
		return false
	}
	_, ok := tp.locData.Translations[tp.locData.Locale]
	return ok
}

// relativeTimeLocalized formats a Unix timestamp as a localized relative time string.
// Falls back to the English default if translations are unavailable.
func (tp *templateParser) relativeTimeLocalized(timestamp int64) string {
	currentTime := time.Now().Unix()
	diff := timestamp - currentTime

	if !tp.hasLocalizationData() {
		return relativeTimeDefault(diff)
	}

	diw := tp.locData.Translations[tp.locData.Locale].Datetime.DistanceInWords

	switch {
	case diff < -60*60: // more than an hour ago
		count := -diff / 3600
		expr := tp.localizedDistanceInWords(diw.AboutXHours.One, diw.AboutXHours.Other, count)
		if expr == "" {
			return relativeTimeDefault(diff)
		}
		return tp.wrapRelative(expr, false)

	case diff < -60*2: // more than 2 minutes ago
		count := -diff / 60
		expr := tp.localizedDistanceInWords(diw.XMinutes.One, diw.XMinutes.Other, count)
		if expr == "" {
			return relativeTimeDefault(diff)
		}
		return tp.wrapRelative(expr, false)

	case diff < -90: // more than 90 seconds ago
		count := -diff
		expr := tp.localizedDistanceInWords(diw.XSeconds.One, diw.XSeconds.Other, count)
		if expr == "" {
			return relativeTimeDefault(diff)
		}
		return tp.wrapRelative(expr, false)

	case diff < -50: // more than 50 seconds ago
		expr := tp.localizedDistanceInWords(diw.XMinutes.One, diw.XMinutes.Other, 1)
		if expr == "" {
			return relativeTimeDefault(diff)
		}
		return tp.wrapRelative(expr, false)

	case diff < -5: // more than 5 seconds ago
		count := -diff
		expr := tp.localizedDistanceInWords(diw.XSeconds.One, diw.XSeconds.Other, count)
		if expr == "" {
			return relativeTimeDefault(diff)
		}
		return tp.wrapRelative(expr, false)

	case diff <= 0: // in the last 5 seconds
		expr := tp.localizedDistanceInWords(diw.LessThanXSeconds.One, diw.LessThanXSeconds.Other, 1)
		if expr == "" {
			return relativeTimeDefault(diff)
		}
		return tp.wrapRelative(expr, false)

	case diff < 60*10: // less than 10 minutes
		expr := tp.localizedDistanceInWords(diw.LessThanXMinutes.One, diw.LessThanXMinutes.Other, 1)
		if expr == "" {
			return relativeTimeDefault(diff)
		}
		return tp.wrapRelative(expr, true)

	case diff < 60*50: // less than 50 minutes
		count := diff / 60
		expr := tp.localizedDistanceInWords(diw.XMinutes.One, diw.XMinutes.Other, count)
		if expr == "" {
			return relativeTimeDefault(diff)
		}
		return tp.wrapRelative(expr, true)

	case diff < 60*90: // less than 90 minutes
		expr := tp.localizedDistanceInWords(diw.AboutXHours.One, diw.AboutXHours.Other, 1)
		if expr == "" {
			return relativeTimeDefault(diff)
		}
		return tp.wrapRelative(expr, true)

	case diff < 60*60*2: // less than 2 hours
		count := diff / 60
		expr := tp.localizedDistanceInWords(diw.XMinutes.One, diw.XMinutes.Other, count)
		if expr == "" {
			return relativeTimeDefault(diff)
		}
		return tp.wrapRelative(expr, true)

	case diff < 60*60*23: // less than 23 hours
		count := diff / 3600
		expr := tp.localizedDistanceInWords(diw.AboutXHours.One, diw.AboutXHours.Other, count)
		if expr == "" {
			return relativeTimeDefault(diff)
		}
		return tp.wrapRelative(expr, true)

	case diff < 60*60*36: // less than 36 hours
		expr := tp.localizedDistanceInWords(diw.XDays.One, diw.XDays.Other, 1)
		if expr == "" {
			return relativeTimeDefault(diff)
		}
		return tp.wrapRelative(expr, true)

	case diff < 60*60*48: // less than 48 hours
		count := diff / 3600
		expr := tp.localizedDistanceInWords(diw.AboutXHours.One, diw.AboutXHours.Other, count)
		if expr == "" {
			return relativeTimeDefault(diff)
		}
		return tp.wrapRelative(expr, true)

	case diff < 60*60*24*14: // less than 14 days
		count := diff / 86400
		expr := tp.localizedDistanceInWords(diw.XDays.One, diw.XDays.Other, count)
		if expr == "" {
			return relativeTimeDefault(diff)
		}
		return tp.wrapRelative(expr, true)

	default: // 2 weeks or more -- express as days since no x_weeks translation key exists
		count := diff / 86400
		expr := tp.localizedDistanceInWords(diw.XDays.One, diw.XDays.Other, count)
		if expr == "" {
			return relativeTimeDefault(diff)
		}
		return tp.wrapRelative(expr, true)
	}
}

// relativeTimeDefault is the original English-only implementation used as a fallback.
func relativeTimeDefault(diff int64) string {
	switch {
	case diff < -60*60:
		return fmt.Sprintf("%d Hours Ago", -diff/3600)
	case diff < -60*2:
		return fmt.Sprintf("%d Minutes Ago", -diff/60)
	case diff < -90:
		return fmt.Sprintf("%d Seconds Ago", -diff)
	case diff < -50:
		return "One Minute Ago"
	case diff < -5:
		return fmt.Sprintf("%d Seconds Ago", -diff)
	case diff <= 0:
		return "Just Now"
	case diff < 60*10:
		return "Very Soon"
	case diff < 60*50:
		return fmt.Sprintf("In %d Minutes", diff/60)
	case diff < 60*90:
		return "In About An Hour"
	case diff < 60*60*2:
		return fmt.Sprintf("In %d Minutes", diff/60)
	case diff < 60*60*23:
		return fmt.Sprintf("In %d Hours", diff/3600)
	case diff < 60*60*36:
		return "In One Day"
	case diff < 60*60*48:
		return fmt.Sprintf("In %d Hours", diff/3600)
	case diff < 60*60*24*14:
		return fmt.Sprintf("In %d Days", diff/86400)
	default:
		return fmt.Sprintf("In %d Weeks", diff/604800)
	}
}

// Parse parses text as a template body for the menu template data
// if an error occurs while parsing, an empty string is returned along with the error
func (tp *templateParser) Parse(text string) (string, error) {
	if tp == nil || tp.td == nil {
		return "", errors.New("templateData is nil")
	}

	t, err := template.New("menu_template").Funcs(template.FuncMap{
		funcHasCapability: func(capability string) bool {
			switch capability {
			case funcRelativeTime:
				return true
			case errorlessTemplateVars:
				return true
			case errorlessActions:
				return true
			case circleDot:
				return true
			}
			return false
		},
		funcRelativeTime: tp.relativeTimeLocalized,
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
