package simulator

type FakeHost interface {
	RunQuery(sql string) (results []map[string]string, err error)
}
