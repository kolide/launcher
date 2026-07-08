package dtainfoconsumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/kolide/launcher/v2/ee/agent/types"
)

const (
	dtaFilePrefix string = "data-"
	dtaFileSuffix string = ".dta"
)

type DTAInfoConsumer struct {
	slogger  *slog.Logger
	basePath func() string // easy test stubbing, the path for the dta file
}

func New(knapsack types.Knapsack) *DTAInfoConsumer {
	return &DTAInfoConsumer{
		slogger:  knapsack.Slogger().With("component", "dta_info_consumer"),
		basePath: knapsack.RootDirectory,
	}
}

// Supports both {} and {"dta_blob": <jwt>}. The former is backwards compatible,
// happening without a key.
type dtaPayload struct {
	DTABlob *string `json:"dta_blob"`
}

// Updates the dta on disk. Discards data which fails to parse to avoid retries which cannot succeed.
// Only write errors will bubble up a failure.
func (c *DTAInfoConsumer) Update(data io.Reader) error {
	c.slogger.Log(context.TODO(), slog.LevelDebug, "received dta update")
	var payload dtaPayload
	if err := json.NewDecoder(data).Decode(&payload); err != nil {
		c.slogger.Log(
			context.TODO(),
			slog.LevelDebug,
			"failed to decode dta in Update",
			"err", err,
		)
		return nil
	}

	if payload.DTABlob == nil {
		// expected when no key is active
		c.slogger.Log(context.TODO(), slog.LevelDebug, "dta blob was absent from payload")
		return nil
	}

	munemo, err := parseDta(*payload.DTABlob)
	if err != nil {
		c.slogger.Log(
			context.TODO(),
			slog.LevelWarn,
			"failed to parse dta, will not retry",
			"err", err,
			"dta", *payload.DTABlob,
		)
		return nil
	}

	path := c.dtaFilePath(munemo)
	if err := os.WriteFile(path, []byte(*payload.DTABlob), 0644); err != nil {
		return fmt.Errorf("failed to write dta to %q: %w", path, err)
	}

	c.slogger.Log(context.TODO(), slog.LevelInfo, "successfully wrote dta", "path", path)
	return nil
}

func (c *DTAInfoConsumer) dtaFilePath(munemo string) string {
	filename := dtaFilePrefix + munemo + dtaFileSuffix
	return filepath.Join(c.basePath(), filename)
}

// Pulls the only field required for writing the dta to the right location, its
// munemo, for the file name. We do not validate the JWT or pull other fields.
func parseDta(data string) (string, error) {
	token, _, err := new(jwt.Parser).ParseUnverified(data, jwt.MapClaims{})
	if err != nil {
		return "", err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("no claims in dta")
	}

	munemo, ok := claims["munemo"]
	if !ok {
		return "", errors.New("dta did not claim a munemo")
	}

	if munemo, ok := munemo.(string); ok {
		return munemo, nil
	}

	return "", fmt.Errorf("munemo claim in dta was not a string: %+v", munemo)
}
