package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_PluralForms_Select(t *testing.T) {
	t.Parallel()

	forms := PluralForms{
		Zero:  "zero form",
		One:   "one form",
		Two:   "two form",
		Few:   "few form",
		Many:  "many form",
		Other: "other form",
	}

	tests := []struct {
		name   string
		locale string
		count  int64
		expect string
	}{
		// English: only uses one/other
		{name: "en_1", locale: "en", count: 1, expect: "one form"},
		{name: "en_0", locale: "en", count: 0, expect: "other form"},
		{name: "en_2", locale: "en", count: 2, expect: "other form"},
		{name: "en_5", locale: "en", count: 5, expect: "other form"},

		// Russian: one / few / many / other
		{name: "ru_1", locale: "ru", count: 1, expect: "one form"},
		{name: "ru_2", locale: "ru", count: 2, expect: "few form"},
		{name: "ru_3", locale: "ru", count: 3, expect: "few form"},
		{name: "ru_4", locale: "ru", count: 4, expect: "few form"},
		{name: "ru_5", locale: "ru", count: 5, expect: "many form"},
		{name: "ru_11", locale: "ru", count: 11, expect: "many form"},
		{name: "ru_14", locale: "ru", count: 14, expect: "many form"},
		{name: "ru_21", locale: "ru", count: 21, expect: "one form"},
		{name: "ru_22", locale: "ru", count: 22, expect: "few form"},
		{name: "ru_25", locale: "ru", count: 25, expect: "many form"},
		{name: "ru_100", locale: "ru", count: 100, expect: "many form"},
		{name: "ru_101", locale: "ru", count: 101, expect: "one form"},

		// Arabic: zero / one / two / few / many / other
		{name: "ar_0", locale: "ar", count: 0, expect: "zero form"},
		{name: "ar_1", locale: "ar", count: 1, expect: "one form"},
		{name: "ar_2", locale: "ar", count: 2, expect: "two form"},
		{name: "ar_3", locale: "ar", count: 3, expect: "few form"},
		{name: "ar_10", locale: "ar", count: 10, expect: "few form"},
		{name: "ar_11", locale: "ar", count: 11, expect: "many form"},
		{name: "ar_99", locale: "ar", count: 99, expect: "many form"},
		{name: "ar_100", locale: "ar", count: 100, expect: "other form"},

		// Polish: one / few / many / other
		{name: "pl_1", locale: "pl", count: 1, expect: "one form"},
		{name: "pl_2", locale: "pl", count: 2, expect: "few form"},
		{name: "pl_5", locale: "pl", count: 5, expect: "many form"},
		{name: "pl_12", locale: "pl", count: 12, expect: "many form"},
		{name: "pl_22", locale: "pl", count: 22, expect: "few form"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := forms.Select(tt.locale, tt.count)
			require.Equal(t, tt.expect, result)
		})
	}
}

func Test_PluralForms_Select_FallbackToOther(t *testing.T) {
	t.Parallel()

	forms := PluralForms{
		One:   "one form",
		Other: "other form",
	}

	result := forms.Select("ru", 5)
	require.Equal(t, "other form", result, "should fall back to Other when Many field is empty")
}

func Test_PluralForms_Select_UnknownLocale(t *testing.T) {
	t.Parallel()

	forms := PluralForms{
		One:   "one form",
		Other: "other form",
	}

	result := forms.Select("xx-unknown", 1)
	require.Equal(t, "other form", result, "unknown locale should fall back to Other")
}
