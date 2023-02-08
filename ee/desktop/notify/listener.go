package notify

import (
	"fmt"

	"github.com/go-kit/kit/log"
)

type listener interface {
	Listen() error
	Interrupt(err error)
}

func NewListener(logger log.Logger) (listener, error) {
	l, err := newOsSpecificListener(logger)
	if err != nil {
		return nil, fmt.Errorf("could not create listener for OS: %w", err)
	}

	return l, nil
}
