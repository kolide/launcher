package types

// Translations is an interface for translations data.
type Translations interface {
	Translations() TranslationsData
}

type TranslationsData struct {
	Locale       string `json:"locale"`
	Translations struct {
		Datetime struct {
			DistanceInWords struct {
				AboutXHours struct {
					One   string `json:"one"`
					Other string `json:"other"`
				} `json:"about_x_hours"`
				AboutXMonths struct {
					One   string `json:"one"`
					Other string `json:"other"`
				} `json:"about_x_months"`
				AboutXYears struct {
					One   string `json:"one"`
					Other string `json:"other"`
				} `json:"about_x_years"`
				AlmostXYears struct {
					One   string `json:"one"`
					Other string `json:"other"`
				} `json:"almost_x_years"`
				HalfAMinute      string `json:"half_a_minute"`
				LessThanXSeconds struct {
					One   string `json:"one"`
					Other string `json:"other"`
				} `json:"less_than_x_seconds"`
				LessThanXMinutes struct {
					One   string `json:"one"`
					Other string `json:"other"`
				} `json:"less_than_x_minutes"`
				OverXYears struct {
					One   string `json:"one"`
					Other string `json:"other"`
				} `json:"over_x_years"`
				XSeconds struct {
					One   string `json:"one"`
					Other string `json:"other"`
				} `json:"x_seconds"`
				XMinutes struct {
					One   string `json:"one"`
					Other string `json:"other"`
				} `json:"x_minutes"`
				XDays struct {
					One   string `json:"one"`
					Other string `json:"other"`
				} `json:"x_days"`
				XMonths struct {
					One   string `json:"one"`
					Other string `json:"other"`
				} `json:"x_months"`
				XYears struct {
					One   string `json:"one"`
					Other string `json:"other"`
				} `json:"x_years"`
			} `json:"distance_in_words"`
			Prompts struct {
				Second string `json:"second"`
				Minute string `json:"minute"`
				Hour   string `json:"hour"`
				Day    string `json:"day"`
				Month  string `json:"month"`
				Year   string `json:"year"`
			} `json:"prompts"`
			Relative struct {
				Future string `json:"future"`
				Past   string `json:"past"`
			} `json:"relative"`
		} `json:"datetime"`
	} `json:"translations"`
}
