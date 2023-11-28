package types

type Querier interface {
	Query(query string) ([]map[string]string, error)
	QuerierHealthy() error
}
