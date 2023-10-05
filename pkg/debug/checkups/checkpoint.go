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
		logger    logger
		knapsack  types.Knapsack
		interrupt chan struct{}
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
	ticker := time.NewTicker(time.Minute * 2) // TODO put back to 60
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
	c.interrupt <- struct{}{}
}

func (c *checkPointer) Once(ctx context.Context) {
	checkupData := make([]interface{}, 0)
	checkups := checkupsFor(c.knapsack, logSupported)

	for _, checkup := range checkups {
		logField := strings.ReplaceAll(strings.ToLower(checkup.Name()), " ", "_")
		checkup.Run(ctx, io.Discard)

		summary := c.summarizeData(checkup.Data())
		checkupData = append(checkupData, logField, summary)
		// c.logger.Log(logField, summary)
	}

	c.logger.Log(checkupData...)

	// populate and log the queried static info

	// c.logger.Log("keyinfo", agentKeyInfo())
	// c.logOsqueryInfo()
	// c.logKolideServerVersion()
	// c.logger.Log("connections", c.Connections())
	// c.logger.Log("ip look ups", c.IpLookups())
	// if !c.knapsack.KolideHosted() {
	// 	return
	// }

	// c.logServerProvidedData()
}

// func (c *checkPointer) logDesktopProcs() {
// 	c.logger.Log("user_desktop_processes", runner.InstanceDesktopProcessRecords())
// }

func (c *checkPointer) summarizeData(data map[string]any) string {
	summary := make([]string, 0)
	for k, v := range data {
		switch knownValue := v.(type) {
		case []string:
			summary = append(summary, fmt.Sprintf("%s: [%s]", k, strings.Join(knownValue, ",")))
		case string:
			summary = append(summary, fmt.Sprintf("%s: %s", k, knownValue))
		case map[string]any:
			summary = append(summary, c.summarizeData(knownValue))
		default:
			// if additional nested types are required they should be added above
			summary = append(summary, fmt.Sprintf("unknown type %T requested for key %s on value %v", knownValue, k, v))
		}
	}

	return strings.Join(summary, ",")
}
