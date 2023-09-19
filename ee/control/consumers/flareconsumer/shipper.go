package flareconsumer

import (
	"io"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/debug/shipping"
)

type Shipper struct{}

func (s *Shipper) Ship(loggger log.Logger, k types.Knapsack, flareStream io.Reader) error {
	return shipping.Ship(loggger, k, flareStream)
}
