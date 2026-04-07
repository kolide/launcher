package types

import (
	"golang.org/x/text/feature/plural"
	"golang.org/x/text/language"
)

// Localizer is an interface for translations data.
type Localizer interface {
	LocalizationData() LocalizationData
}

type LocalizationData struct {
	Locale       string                  `json:"locale"`
	Translations map[string]Translations `json:"translations"`
}

type Translations struct {
	Datetime Datetime `json:"datetime"`
}

// PluralForms holds translations for all CLDR plural categories.
// Languages vary in which categories they use -- for example, English uses
// only One/Other, Russian uses One/Few/Many/Other, and Arabic uses all six.
type PluralForms struct {
	Zero  string `json:"zero,omitempty"`
	One   string `json:"one,omitempty"`
	Two   string `json:"two,omitempty"`
	Few   string `json:"few,omitempty"`
	Many  string `json:"many,omitempty"`
	Other string `json:"other"`
}

// Select returns the translation string matching the CLDR plural category for
// the given locale and integer count. It falls back through the chain:
// exact CLDR category -> Other -> empty string.
func (pf PluralForms) Select(locale string, count int64) string {
	tag, _ := language.Parse(locale)
	// MatchPlural args: integer digits, visible fraction digits, num fractional digits,
	// integer value of visible fraction, integer value of fraction w/o trailing zeros
	form := plural.Cardinal.MatchPlural(tag, int(count), 0, 0, 0, 0)

	switch form {
	case plural.Zero:
		if pf.Zero != "" {
			return pf.Zero
		}
	case plural.One:
		if pf.One != "" {
			return pf.One
		}
	case plural.Two:
		if pf.Two != "" {
			return pf.Two
		}
	case plural.Few:
		if pf.Few != "" {
			return pf.Few
		}
	case plural.Many:
		if pf.Many != "" {
			return pf.Many
		}
	}

	return pf.Other
}

type Datetime struct {
	DistanceInWords struct {
		AboutXHours      PluralForms `json:"about_x_hours"`
		AboutXMonths     PluralForms `json:"about_x_months"`
		AboutXYears      PluralForms `json:"about_x_years"`
		AlmostXYears     PluralForms `json:"almost_x_years"`
		HalfAMinute      string      `json:"half_a_minute"`
		LessThanXSeconds PluralForms `json:"less_than_x_seconds"`
		LessThanXMinutes PluralForms `json:"less_than_x_minutes"`
		OverXYears       PluralForms `json:"over_x_years"`
		XSeconds         PluralForms `json:"x_seconds"`
		XMinutes         PluralForms `json:"x_minutes"`
		XDays            PluralForms `json:"x_days"`
		XMonths          PluralForms `json:"x_months"`
		XYears           PluralForms `json:"x_years"`
	} `json:"distance_in_words"`
	Relative struct {
		Future string `json:"future"`
		Past   string `json:"past"`
	} `json:"relative"`
}
