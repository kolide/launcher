package types

// Knapsack is an inventory of data and useful services which are used throughout
// launcher code and are typically valid for the lifetime of the launcher application instance.
type Knapsack interface {
	Stores
	BboltDB
	Flags
}
