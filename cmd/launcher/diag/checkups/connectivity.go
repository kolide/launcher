package checkups

import (
	"fmt"
	"io"
	"strconv"

	"github.com/go-kit/kit/log"

	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/log/checkpoint"
)

type Connectivity struct {
	Logger log.Logger
	K      types.Knapsack
}

func (c *Connectivity) Name() string {
	return "Check communication with Kolide"
}

func (c *Connectivity) Run(short io.Writer) (string, error) {
	return checkupConnectivity(short, c.Logger, c.K)
}

// checkupConnectivity tests connections to Kolide cloud services
func checkupConnectivity(short io.Writer, logger log.Logger, k types.Knapsack) (string, error) {
	var failures int
	checkpointer := checkpoint.New(logger, k)
	connections := checkpointer.Connections()
	for k, v := range connections {
		if v != "successful tcp connection" {
			fail(short, fmt.Sprintf("%s\t%s", k, v))
			failures = failures + 1
			continue
		}
		pass(short, fmt.Sprintf("%s\t%s", k, v))
	}

	ipLookups := checkpointer.IpLookups()
	for k, v := range ipLookups {
		valStrSlice, ok := v.([]string)
		if !ok || len(valStrSlice) == 0 {
			fail(short, fmt.Sprintf("%s\t%s", k, valStrSlice))
			failures = failures + 1
			continue
		}
		pass(short, fmt.Sprintf("%s\t%s", k, valStrSlice))
	}

	notaryVersions, err := checkpointer.NotaryVersions()
	if err != nil {
		fail(short, fmt.Errorf("could not fetch notary versions: %w", err))
		failures = failures + 1
	}

	for k, v := range notaryVersions {
		// Check for failure if the notary version isn't a parsable integer
		if _, err := strconv.ParseInt(v, 10, 32); err != nil {
			fail(short, fmt.Sprintf("%s\t%s", k, v))
			failures = failures + 1
			continue
		}
		pass(short, fmt.Sprintf("%s\t%s", k, v))
	}

	if failures == 0 {
		return "Successfully communicated with Kolide", nil
	}

	return "", fmt.Errorf("%d failures encountered while attempting communication with Kolide", failures)
}
