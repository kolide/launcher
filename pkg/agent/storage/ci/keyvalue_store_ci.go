package storageci

import (
	"os"
	"testing"

	"github.com/go-kit/kit/log"
	agentbbolt "github.com/kolide/launcher/pkg/agent/storage/bbolt"
	"github.com/kolide/launcher/pkg/agent/storage/inmemory"
	"github.com/kolide/launcher/pkg/agent/types"
)

func NewStore(t *testing.T, logger log.Logger, bucketName string) (types.KVStore, error) {
	if os.Getenv("CI") == "true" {
		return inmemory.NewStore(logger), nil
	}

	return agentbbolt.NewStore(logger, agentbbolt.SetupDB(t), bucketName)
}
