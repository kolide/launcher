package keyidentifier

func truePtr() *bool {
	return boolPtr(true)
}

func falsePtr() *bool {
	return boolPtr(false)
}

func boolPtr(b bool) *bool {
	return &b
}
