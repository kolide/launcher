package menu

import (
	"fmt"
	"testing"
	"time"

	"github.com/kolide/launcher/v2/ee/agent/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Parse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		td          *TemplateData
		text        string
		output      string
		expectedErr bool
	}{
		{
			name:   "default",
			td:     &TemplateData{},
			text:   "Version: {{.LauncherVersion}}",
			output: "Version: <no value>",
		},
		{
			name:   "single option",
			td:     &TemplateData{ServerHostname: "localhost"},
			text:   "Hostname: {{.ServerHostname}}",
			output: "Hostname: localhost",
		},
		{
			name:   "multiple option",
			td:     &TemplateData{ServerHostname: "localhost", LauncherVersion: "0.0.0"},
			text:   "Hostname: {{.ServerHostname}}, launcher version: {{.LauncherVersion}}",
			output: "Hostname: localhost, launcher version: 0.0.0",
		},
		{
			name:   "unsupported capability",
			td:     &TemplateData{ServerHostname: "localhost", LauncherVersion: "0.0.0"},
			text:   "This capability is {{if hasCapability `bad capability`}}supported{{else}}unsupported{{end}}.",
			output: "This capability is unsupported.",
		},
		{
			name:   "circleDot capability",
			td:     &TemplateData{},
			text:   "\"icon\":\"{{if not (hasCapability `circleDot`)}}triangle-exclamation{{else}}circle-dot{{end}}\"",
			output: "\"icon\":\"circle-dot\"",
		},
		{
			name:   "icon fallback capability",
			td:     &TemplateData{},
			text:   "\"icon\":\"{{if not (hasCapability `asOfYetUnknownIconType`)}}triangle-exclamation{{else}}new-icon-type{{end}}\"",
			output: "\"icon\":\"triangle-exclamation\"",
		},
		{
			name:   "relativeTime 2 hours ago",
			td:     &TemplateData{LastMenuUpdateTime: time.Now().Add(-2 * time.Hour).Unix()},
			text:   "This Menu Was Last Updated {{if hasCapability `relativeTime`}}{{relativeTime .LastMenuUpdateTime}}{{else}}never{{end}}.",
			output: "This Menu Was Last Updated 2 Hours Ago.",
		},
		{
			name:   "relativeTime 15 minutes ago",
			td:     &TemplateData{LastMenuUpdateTime: time.Now().Add(-15*time.Minute - 30*time.Second).Unix()},
			text:   "This Menu Was Last Updated {{if hasCapability `relativeTime`}}{{relativeTime .LastMenuUpdateTime}}{{else}}never{{end}}.",
			output: "This Menu Was Last Updated 15 Minutes Ago.",
		},
		{
			name:   "relativeTime one minute ago",
			td:     &TemplateData{LastMenuUpdateTime: time.Now().Add(-1 * time.Minute).Unix()},
			text:   "This Menu Was Last Updated {{if hasCapability `relativeTime`}}{{relativeTime .LastMenuUpdateTime}}{{else}}never{{end}}.",
			output: "This Menu Was Last Updated One Minute Ago.",
		},
		{
			name:   "relativeTime one second",
			td:     &TemplateData{LastMenuUpdateTime: time.Now().Add(-1 * time.Second).Unix()},
			text:   "This Menu Was Last Updated {{if hasCapability `relativeTime`}}{{relativeTime .LastMenuUpdateTime}}{{else}}never{{end}}.",
			output: "This Menu Was Last Updated Just Now.",
		},
		{
			name:   "relativeTime just now",
			td:     &TemplateData{LastMenuUpdateTime: time.Now().Unix()},
			text:   "This Menu Was Last Updated {{if hasCapability `relativeTime`}}{{relativeTime .LastMenuUpdateTime}}{{else}}never{{end}}.",
			output: "This Menu Was Last Updated Just Now.",
		},
		{
			name:   "relativeTime very soon",
			td:     &TemplateData{ServerHostname: "localhost", LauncherVersion: "0.0.0"},
			text:   fmt.Sprintf("This Is Starting {{if hasCapability `relativeTime`}}{{relativeTime %d}}{{else}}never{{end}}.", time.Now().Add(2*time.Minute+30*time.Second).Unix()),
			output: "This Is Starting Very Soon.",
		},
		{
			name:   "relativeTime 15 minutes",
			td:     &TemplateData{ServerHostname: "localhost", LauncherVersion: "0.0.0"},
			text:   fmt.Sprintf("This Is Starting {{if hasCapability `relativeTime`}}{{relativeTime %d}}{{else}}never{{end}}.", time.Now().Add(15*time.Minute+30*time.Second).Unix()),
			output: "This Is Starting In 15 Minutes.",
		},
		{
			name:   "relativeTime one hour",
			td:     &TemplateData{ServerHostname: "localhost", LauncherVersion: "0.0.0"},
			text:   fmt.Sprintf("This Is Starting {{if hasCapability `relativeTime`}}{{relativeTime %d}}{{else}}never{{end}}.", time.Now().Add(1*time.Hour+30*time.Second).Unix()),
			output: "This Is Starting In About An Hour.",
		},
		{
			name:   "relativeTime 111 minutes",
			td:     &TemplateData{ServerHostname: "localhost", LauncherVersion: "0.0.0"},
			text:   fmt.Sprintf("This Is Starting {{if hasCapability `relativeTime`}}{{relativeTime %d}}{{else}}never{{end}}.", time.Now().Add(111*time.Minute+30*time.Second).Unix()),
			output: "This Is Starting In 111 Minutes.",
		},
		{
			name:   "relativeTime four hours",
			td:     &TemplateData{ServerHostname: "localhost", LauncherVersion: "0.0.0"},
			text:   fmt.Sprintf("This Is Starting {{if hasCapability `relativeTime`}}{{relativeTime %d}}{{else}}never{{end}}.", time.Now().Add(4*time.Hour+30*time.Second).Unix()),
			output: "This Is Starting In 4 Hours.",
		},
		{
			name:   "relativeTime one day",
			td:     &TemplateData{ServerHostname: "localhost", LauncherVersion: "0.0.0"},
			text:   fmt.Sprintf("This Is Starting {{if hasCapability `relativeTime`}}{{relativeTime %d}}{{else}}never{{end}}.", time.Now().Add(24*time.Hour+30*time.Second).Unix()),
			output: "This Is Starting In One Day.",
		},
		{
			name:   "relativeTime 44 hours",
			td:     &TemplateData{ServerHostname: "localhost", LauncherVersion: "0.0.0"},
			text:   fmt.Sprintf("This Is Starting {{if hasCapability `relativeTime`}}{{relativeTime %d}}{{else}}never{{end}}.", time.Now().Add(44*time.Hour+30*time.Second).Unix()),
			output: "This Is Starting In 44 Hours.",
		},
		{
			name:   "relativeTime three days",
			td:     &TemplateData{ServerHostname: "localhost", LauncherVersion: "0.0.0"},
			text:   fmt.Sprintf("This Is Starting {{if hasCapability `relativeTime`}}{{relativeTime %d}}{{else}}never{{end}}.", time.Now().Add(3*24*time.Hour+30*time.Second).Unix()),
			output: "This Is Starting In 3 Days.",
		},
		{
			name:   "relativeTime seven days",
			td:     &TemplateData{ServerHostname: "localhost", LauncherVersion: "0.0.0"},
			text:   fmt.Sprintf("This Is Starting {{if hasCapability `relativeTime`}}{{relativeTime %d}}{{else}}never{{end}}.", time.Now().Add(7*24*time.Hour+30*time.Second).Unix()),
			output: "This Is Starting In 7 Days.",
		},
		{
			name:   "relativeTime two weeks",
			td:     &TemplateData{ServerHostname: "localhost", LauncherVersion: "0.0.0"},
			text:   fmt.Sprintf("This Is Starting {{if hasCapability `relativeTime`}}{{relativeTime %d}}{{else}}never{{end}}.", time.Now().Add(14*24*time.Hour+30*time.Second).Unix()),
			output: "This Is Starting In 2 Weeks.",
		},
		{
			name:   "errorlessTemplateVars",
			td:     &TemplateData{MenuVersion: CurrentMenuVersion},
			text:   "{{if hasCapability `errorlessTemplateVars`}}{{.MenuVersion}}{{else}}{{end}}",
			output: CurrentMenuVersion,
		},
		{
			name:        "unparseable error",
			td:          &TemplateData{},
			text:        "Version: {{GARBAGE}}",
			output:      "",
			expectedErr: true,
		},
		{
			name:   "undefined key",
			td:     &TemplateData{},
			text:   "Version: {{.UndefinedKey}}",
			output: "Version: <no value>",
		},
		{
			name:   "undefined key with value check",
			td:     &TemplateData{},
			text:   "{{if .UndefinedKey}}UndefinedKey is: {{.UndefinedKey}}{{else}}UndefinedKey is NOT set.{{end}}",
			output: "UndefinedKey is NOT set.",
		},
		{
			name:   "current menu version",
			td:     &TemplateData{MenuVersion: CurrentMenuVersion},
			text:   "{{if .MenuVersion}}Menu Version is: {{.MenuVersion}}{{else}}{{end}}",
			output: fmt.Sprintf("Menu Version is: %s", CurrentMenuVersion),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tp := NewTemplateParser(tt.td, types.LocalizationData{})
			o, err := tp.Parse(tt.text)
			if tt.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.output, o)
		})
	}
}

func Test_Parse_Seconds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		td           *TemplateData
		text         string
		outputFormat string
		seconds      int
	}{
		{
			name:         "relativeTime 111 seconds ago",
			td:           &TemplateData{LastMenuUpdateTime: time.Now().Add(-111 * time.Second).Unix()},
			text:         "This Menu Was Last Updated {{if hasCapability `relativeTime`}}{{relativeTime .LastMenuUpdateTime}}{{else}}never{{end}}.",
			outputFormat: "This Menu Was Last Updated %d Seconds Ago.",
			seconds:      111,
		},
		{
			name:         "relativeTime 7 seconds ago",
			td:           &TemplateData{LastMenuUpdateTime: time.Now().Add(-7 * time.Second).Unix()},
			text:         "This Menu Was Last Updated {{if hasCapability `relativeTime`}}{{relativeTime .LastMenuUpdateTime}}{{else}}never{{end}}.",
			outputFormat: "This Menu Was Last Updated %d Seconds Ago.",
			seconds:      7,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tp := NewTemplateParser(tt.td, types.LocalizationData{})
			o, err := tp.Parse(tt.text)
			require.NoError(t, err)

			// Sometimes we're off by a second and that's fine.
			expectedOutput := fmt.Sprintf(tt.outputFormat, tt.seconds)
			expectedOutputPlusOneSecond := fmt.Sprintf(tt.outputFormat, tt.seconds+1)
			expectedOutputMinusOneSecond := fmt.Sprintf(tt.outputFormat, tt.seconds-1)
			require.True(t,
				o == expectedOutput ||
					o == expectedOutputPlusOneSecond ||
					o == expectedOutputMinusOneSecond,
				fmt.Sprintf("expected output %s to be within one second of %d but was not", o, tt.seconds),
			)
		})
	}
}

func spanishLocData() types.LocalizationData {
	var dt types.Datetime
	dt.DistanceInWords.AboutXHours.One = "alrededor de %{count} hora"
	dt.DistanceInWords.AboutXHours.Other = "alrededor de %{count} horas"
	dt.DistanceInWords.LessThanXSeconds.One = "menos de %{count} segundo"
	dt.DistanceInWords.LessThanXSeconds.Other = "menos de %{count} segundos"
	dt.DistanceInWords.LessThanXMinutes.One = "menos de %{count} minuto"
	dt.DistanceInWords.LessThanXMinutes.Other = "menos de %{count} minutos"
	dt.DistanceInWords.XSeconds.One = "%{count} segundo"
	dt.DistanceInWords.XSeconds.Other = "%{count} segundos"
	dt.DistanceInWords.XMinutes.One = "%{count} minuto"
	dt.DistanceInWords.XMinutes.Other = "%{count} minutos"
	dt.DistanceInWords.XDays.One = "%{count} día"
	dt.DistanceInWords.XDays.Other = "%{count} días"
	dt.Relative.Future = "en %{time}"
	dt.Relative.Past = "hace %{time}"

	return types.LocalizationData{
		Locale: "es",
		Translations: map[string]types.Translations{
			"es": {
				Datetime: dt,
			},
		},
	}
}

func Test_Parse_Localized(t *testing.T) {
	t.Parallel()

	locData := spanishLocData()

	tests := []struct {
		name   string
		td     *TemplateData
		text   string
		output string
	}{
		{
			name:   "localized 2 hours ago",
			td:     &TemplateData{LastMenuUpdateTime: time.Now().Add(-2 * time.Hour).Unix()},
			text:   "{{relativeTime .LastMenuUpdateTime}}",
			output: "hace alrededor de 2 horas",
		},
		{
			name:   "localized 15 minutes ago",
			td:     &TemplateData{LastMenuUpdateTime: time.Now().Add(-15*time.Minute - 30*time.Second).Unix()},
			text:   "{{relativeTime .LastMenuUpdateTime}}",
			output: "hace 15 minutos",
		},
		{
			name:   "localized one minute ago",
			td:     &TemplateData{LastMenuUpdateTime: time.Now().Add(-1 * time.Minute).Unix()},
			text:   "{{relativeTime .LastMenuUpdateTime}}",
			output: "hace 1 minuto",
		},
		{
			name:   "localized just now",
			td:     &TemplateData{LastMenuUpdateTime: time.Now().Unix()},
			text:   "{{relativeTime .LastMenuUpdateTime}}",
			output: "hace menos de 1 segundo",
		},
		{
			name:   "localized very soon",
			td:     &TemplateData{},
			text:   fmt.Sprintf("{{relativeTime %d}}", time.Now().Add(2*time.Minute+30*time.Second).Unix()),
			output: "en menos de 1 minuto",
		},
		{
			name:   "localized 15 minutes future",
			td:     &TemplateData{},
			text:   fmt.Sprintf("{{relativeTime %d}}", time.Now().Add(15*time.Minute+30*time.Second).Unix()),
			output: "en 15 minutos",
		},
		{
			name:   "localized about one hour",
			td:     &TemplateData{},
			text:   fmt.Sprintf("{{relativeTime %d}}", time.Now().Add(1*time.Hour+30*time.Second).Unix()),
			output: "en alrededor de 1 hora",
		},
		{
			name:   "localized one day",
			td:     &TemplateData{},
			text:   fmt.Sprintf("{{relativeTime %d}}", time.Now().Add(24*time.Hour+30*time.Second).Unix()),
			output: "en 1 día",
		},
		{
			name:   "localized 3 days",
			td:     &TemplateData{},
			text:   fmt.Sprintf("{{relativeTime %d}}", time.Now().Add(3*24*time.Hour+30*time.Second).Unix()),
			output: "en 3 días",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tp := NewTemplateParser(tt.td, locData)
			o, err := tp.Parse(tt.text)
			require.NoError(t, err)
			assert.Equal(t, tt.output, o)
		})
	}
}

func Test_Parse_FallbackOnMissingLocale(t *testing.T) {
	t.Parallel()

	locData := types.LocalizationData{
		Locale:       "zz",
		Translations: map[string]types.Translations{},
	}

	td := &TemplateData{LastMenuUpdateTime: time.Now().Add(-2 * time.Hour).Unix()}
	tp := NewTemplateParser(td, locData)
	o, err := tp.Parse("{{relativeTime .LastMenuUpdateTime}}")
	require.NoError(t, err)
	assert.Equal(t, "2 Hours Ago", o)
}

func Test_localizedDistanceInWords(t *testing.T) {
	t.Parallel()

	tp := &templateParser{
		locData: types.LocalizationData{Locale: "en"},
	}

	tests := []struct {
		name   string
		forms  types.PluralForms
		count  int64
		expect string
	}{
		{
			name:   "basic count replacement",
			forms:  types.PluralForms{One: "%{count} minute", Other: "%{count} minutes"},
			count:  15,
			expect: "15 minutes",
		},
		{
			name:   "singular form",
			forms:  types.PluralForms{One: "%{count} minute", Other: "%{count} minutes"},
			count:  1,
			expect: "1 minute",
		},
		{
			name:   "no placeholder",
			forms:  types.PluralForms{One: "half a minute", Other: "half a minute"},
			count:  1,
			expect: "half a minute",
		},
		{
			name:   "multiple count placeholders",
			forms:  types.PluralForms{One: "%{count} of %{count}", Other: "%{count} of %{count}"},
			count:  3,
			expect: "3 of 3",
		},
		{
			name:   "empty translations return empty string",
			forms:  types.PluralForms{},
			count:  5,
			expect: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := tp.localizedDistanceInWords(tt.forms, tt.count)
			assert.Equal(t, tt.expect, result)
		})
	}
}

func russianLocData() types.LocalizationData {
	return types.LocalizationData{
		Locale: "ru",
		Translations: map[string]types.Translations{
			"ru": {
				Datetime: types.Datetime{
					DistanceInWords: struct {
						AboutXHours      types.PluralForms `json:"about_x_hours"`
						AboutXMonths     types.PluralForms `json:"about_x_months"`
						AboutXYears      types.PluralForms `json:"about_x_years"`
						AlmostXYears     types.PluralForms `json:"almost_x_years"`
						HalfAMinute      string            `json:"half_a_minute"`
						LessThanXSeconds types.PluralForms `json:"less_than_x_seconds"`
						LessThanXMinutes types.PluralForms `json:"less_than_x_minutes"`
						OverXYears       types.PluralForms `json:"over_x_years"`
						XSeconds         types.PluralForms `json:"x_seconds"`
						XMinutes         types.PluralForms `json:"x_minutes"`
						XDays            types.PluralForms `json:"x_days"`
						XMonths          types.PluralForms `json:"x_months"`
						XYears           types.PluralForms `json:"x_years"`
					}{
						AboutXHours: types.PluralForms{
							One: "около %{count} часа", Few: "около %{count} часов",
							Many: "около %{count} часов", Other: "около %{count} часов",
						},
						LessThanXSeconds: types.PluralForms{
							One: "меньше %{count} секунды", Few: "меньше %{count} секунд",
							Many: "меньше %{count} секунд", Other: "меньше %{count} секунд",
						},
						LessThanXMinutes: types.PluralForms{
							One: "меньше %{count} минуты", Few: "меньше %{count} минут",
							Many: "меньше %{count} минут", Other: "меньше %{count} минут",
						},
						XSeconds: types.PluralForms{
							One: "%{count} секунда", Few: "%{count} секунды",
							Many: "%{count} секунд", Other: "%{count} секунд",
						},
						XMinutes: types.PluralForms{
							One: "%{count} минута", Few: "%{count} минуты",
							Many: "%{count} минут", Other: "%{count} минут",
						},
						XDays: types.PluralForms{
							One: "%{count} день", Few: "%{count} дня",
							Many: "%{count} дней", Other: "%{count} дней",
						},
					},
					Relative: struct {
						Future string `json:"future"`
						Past   string `json:"past"`
					}{
						Future: "через %{time}",
						Past:   "%{time} назад",
					},
				},
			},
		},
	}
}

func Test_Parse_Localized_Russian(t *testing.T) {
	t.Parallel()

	locData := russianLocData()

	tests := []struct {
		name   string
		td     *TemplateData
		text   string
		output string
	}{
		{
			name:   "ru 1 minute ago (one)",
			td:     &TemplateData{LastMenuUpdateTime: time.Now().Add(-1 * time.Minute).Unix()},
			text:   "{{relativeTime .LastMenuUpdateTime}}",
			output: "1 минута назад",
		},
		{
			name:   "ru 3 minutes ago (few)",
			td:     &TemplateData{LastMenuUpdateTime: time.Now().Add(-3*time.Minute - 30*time.Second).Unix()},
			text:   "{{relativeTime .LastMenuUpdateTime}}",
			output: "3 минуты назад",
		},
		{
			name:   "ru 5 minutes ago (many)",
			td:     &TemplateData{LastMenuUpdateTime: time.Now().Add(-5*time.Minute - 30*time.Second).Unix()},
			text:   "{{relativeTime .LastMenuUpdateTime}}",
			output: "5 минут назад",
		},
		{
			name:   "ru 21 minutes ago (one, mod-10 rule)",
			td:     &TemplateData{LastMenuUpdateTime: time.Now().Add(-21*time.Minute - 30*time.Second).Unix()},
			text:   "{{relativeTime .LastMenuUpdateTime}}",
			output: "21 минута назад",
		},
		{
			name:   "ru 22 minutes ago (few, mod-10 rule)",
			td:     &TemplateData{LastMenuUpdateTime: time.Now().Add(-22*time.Minute - 30*time.Second).Unix()},
			text:   "{{relativeTime .LastMenuUpdateTime}}",
			output: "22 минуты назад",
		},
		{
			name:   "ru 2 hours ago (few)",
			td:     &TemplateData{LastMenuUpdateTime: time.Now().Add(-2 * time.Hour).Unix()},
			text:   "{{relativeTime .LastMenuUpdateTime}}",
			output: "около 2 часов назад",
		},
		{
			name:   "ru 3 days future (few)",
			td:     &TemplateData{},
			text:   fmt.Sprintf("{{relativeTime %d}}", time.Now().Add(3*24*time.Hour+30*time.Second).Unix()),
			output: "через 3 дня",
		},
		{
			name:   "ru 5 days future (many)",
			td:     &TemplateData{},
			text:   fmt.Sprintf("{{relativeTime %d}}", time.Now().Add(5*24*time.Hour+30*time.Second).Unix()),
			output: "через 5 дней",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tp := NewTemplateParser(tt.td, locData)
			o, err := tp.Parse(tt.text)
			require.NoError(t, err)
			assert.Equal(t, tt.output, o)
		})
	}
}

func arabicLocData() types.LocalizationData {
	return types.LocalizationData{
		Locale: "ar",
		Translations: map[string]types.Translations{
			"ar": {
				Datetime: types.Datetime{
					DistanceInWords: struct {
						AboutXHours      types.PluralForms `json:"about_x_hours"`
						AboutXMonths     types.PluralForms `json:"about_x_months"`
						AboutXYears      types.PluralForms `json:"about_x_years"`
						AlmostXYears     types.PluralForms `json:"almost_x_years"`
						HalfAMinute      string            `json:"half_a_minute"`
						LessThanXSeconds types.PluralForms `json:"less_than_x_seconds"`
						LessThanXMinutes types.PluralForms `json:"less_than_x_minutes"`
						OverXYears       types.PluralForms `json:"over_x_years"`
						XSeconds         types.PluralForms `json:"x_seconds"`
						XMinutes         types.PluralForms `json:"x_minutes"`
						XDays            types.PluralForms `json:"x_days"`
						XMonths          types.PluralForms `json:"x_months"`
						XYears           types.PluralForms `json:"x_years"`
					}{
						AboutXHours: types.PluralForms{
							Zero: "حوالي صفر ساعات", One: "حوالي ساعة واحدة",
							Two: "حوالي ساعتان", Few: "حوالي %{count} ساعات",
							Many: "حوالي %{count} ساعة", Other: "حوالي %{count} ساعة",
						},
						LessThanXSeconds: types.PluralForms{
							Zero: "أقل من صفر ثواني", One: "أقل من ثانية",
							Two: "أقل من ثانيتان", Few: "أقل من %{count} ثوان",
							Many: "أقل من %{count} ثانية", Other: "أقل من %{count} ثانية",
						},
						LessThanXMinutes: types.PluralForms{
							Zero: "أقل من صفر دقائق", One: "أقل من دقيقة",
							Two: "أقل من دقيقتان", Few: "أقل من %{count} دقائق",
							Many: "أقل من %{count} دقيقة", Other: "أقل من %{count} دقيقة",
						},
						XSeconds: types.PluralForms{
							Zero: "صفر ثواني", One: "ثانية واحدة",
							Two: "ثانيتان", Few: "%{count} ثوان",
							Many: "%{count} ثانية", Other: "%{count} ثانية",
						},
						XMinutes: types.PluralForms{
							Zero: "صفر دقائق", One: "دقيقة واحدة",
							Two: "دقيقتان", Few: "%{count} دقائق",
							Many: "%{count} دقيقة", Other: "%{count} دقيقة",
						},
						XDays: types.PluralForms{
							Zero: "صفر أيام", One: "يوم واحد",
							Two: "يومان", Few: "%{count} أيام",
							Many: "%{count} يوم", Other: "%{count} يوم",
						},
					},
					Relative: struct {
						Future string `json:"future"`
						Past   string `json:"past"`
					}{
						Future: "خلال %{time}",
						Past:   "منذ %{time}",
					},
				},
			},
		},
	}
}

func Test_Parse_Localized_Arabic(t *testing.T) {
	t.Parallel()

	locData := arabicLocData()

	tests := []struct {
		name   string
		td     *TemplateData
		text   string
		output string
	}{
		{
			name:   "ar 1 minute ago (one)",
			td:     &TemplateData{LastMenuUpdateTime: time.Now().Add(-1 * time.Minute).Unix()},
			text:   "{{relativeTime .LastMenuUpdateTime}}",
			output: "منذ دقيقة واحدة",
		},
		{
			name:   "ar 3 minutes ago (few, n%100 in 3..10)",
			td:     &TemplateData{LastMenuUpdateTime: time.Now().Add(-3*time.Minute - 30*time.Second).Unix()},
			text:   "{{relativeTime .LastMenuUpdateTime}}",
			output: "منذ 3 دقائق",
		},
		{
			name:   "ar 11 minutes ago (many, n%100 in 11..99)",
			td:     &TemplateData{LastMenuUpdateTime: time.Now().Add(-11*time.Minute - 30*time.Second).Unix()},
			text:   "{{relativeTime .LastMenuUpdateTime}}",
			output: "منذ 11 دقيقة",
		},
		{
			name:   "ar 2 hours ago (two)",
			td:     &TemplateData{LastMenuUpdateTime: time.Now().Add(-2 * time.Hour).Unix()},
			text:   "{{relativeTime .LastMenuUpdateTime}}",
			output: "منذ حوالي ساعتان",
		},
		{
			name:   "ar 3 days future (few)",
			td:     &TemplateData{},
			text:   fmt.Sprintf("{{relativeTime %d}}", time.Now().Add(3*24*time.Hour+30*time.Second).Unix()),
			output: "خلال 3 أيام",
		},
		{
			name:   "ar 11 days future (many)",
			td:     &TemplateData{},
			text:   fmt.Sprintf("{{relativeTime %d}}", time.Now().Add(11*24*time.Hour+30*time.Second).Unix()),
			output: "خلال 11 يوم",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tp := NewTemplateParser(tt.td, locData)
			o, err := tp.Parse(tt.text)
			require.NoError(t, err)
			assert.Equal(t, tt.output, o)
		})
	}
}
