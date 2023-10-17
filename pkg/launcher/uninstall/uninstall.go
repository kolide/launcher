package uninstall

import (
	"errors"
	"os"
	"runtime"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/osquery"
)

func Uninstall(logger log.Logger, knapsack types.Knapsack) {
	logger = log.With(logger, "component", "uninstall")

	if runtime.GOOS != "darwin" {
		level.Info(logger).Log("msg", "uninstall is currently only supported on darwin")
		return
	}

	if err := knapsack.ConfigStore().Delete([]byte(osquery.NodeKeyKey)); err != nil {
		level.Error(logger).Log("msg", "deleting node key", "err", err)
	}

	if err := removeEnrollSecretFile(knapsack); err != nil {
		level.Error(logger).Log("msg", "removing enroll secret", "err", err)
	}

	if err := removeStartScripts(); err != nil {
		level.Error(logger).Log("msg", "removing start scripts", "err", err)
	}

	if err := removeInstallation(); err != nil {
		level.Error(logger).Log("msg", "removing installation", "err", err)
	}

	os.Exit(0)
}

func removeEnrollSecretFile(knapsack types.Knapsack) error {
	if knapsack.EnrollSecretPath() == "" {
		return errors.New("no enroll secret path set")
	}

	if err := os.Remove(knapsack.EnrollSecretPath()); err != nil {
		return err
	}

	return nil
}
