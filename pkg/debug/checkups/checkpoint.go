package checkups

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/agent/types"
)

// logger is an interface that allows mocking of logger
type (
	logger interface {
		Log(keyvals ...interface{}) error
	}

	checkPointer struct {
		logger      logger
		knapsack    types.Knapsack
		interrupt   chan struct{}
		interrupted bool
	}
)

func NewCheckupLogger(logger logger, k types.Knapsack) *checkPointer {
	return &checkPointer{
		logger:    log.With(logger, "component", "log checkpoint"),
		knapsack:  k,
		interrupt: make(chan struct{}, 1),
	}
}

// Run starts a log checkpoint routine. The purpose of this is to
// ensure we get good debugging information in the logs.
func (c *checkPointer) Run() error {
	ticker := time.NewTicker(time.Minute * 60)
	defer ticker.Stop()

	for {
		c.Once(context.TODO())

		select {
		case <-ticker.C:
			continue
		case <-c.interrupt:
			level.Debug(c.logger).Log("msg", "interrupt received, exiting execute loop")
			return nil
		}
	}
}

func (c *checkPointer) Interrupt(_ error) {
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if c.interrupted {
		return
	}

	c.interrupted = true

	c.interrupt <- struct{}{}
}

func (c *checkPointer) Once(ctx context.Context) {
	checkups := checkupsFor(c.knapsack, logSupported)

	for _, checkup := range checkups {
		checkup.Run(ctx, io.Discard)

		logValues := []interface{}{"checkup", checkup.Name()}
		for k, v := range checkup.Data() {
			logValues = append(logValues, k, c.summarizeData(v))
		}

		c.logger.Log(logValues...)
	}
}

func (c *checkPointer) summarizeData(data any) any {
	switch knownValue := data.(type) {
	case []string:
		return strings.Join(knownValue, ",")
	case string, uint, int, int32, int64:
		return knownValue
	default:
		return fmt.Sprintf("%v", data)
	}
}
