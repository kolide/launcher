package secureboot

import "github.com/go-kit/kit/log"

type Table struct {
	logger log.Logger
	cmd    string
	args   []string
}
