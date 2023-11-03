package table

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/dgrijalva/jwt-go"
	"github.com/go-kit/kit/log"
	"github.com/osquery/osquery-go/plugin/table"
)

type JWTTable struct {
	logger log.Logger
}

func JWT(logger log.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("path"),
		table.TextColumn("claims"),
		table.TextColumn("error"),
	}

	jwt := &JWTTable{
		logger: logger,
	}

	return table.NewPlugin("kolide_jwt", columns, jwt.generate)
}

func (j *JWTTable) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	q, ok := queryContext.Constraints["path"]
	if !ok || len(q.Constraints) == 0 {
		return results, errors.New("the kolide_jwt table requires that you specify a constraint for path")
	}

	for _, constraint := range q.Constraints {
		path := constraint.Expression
		res := map[string]string{
			"path":   path,
			"error":  "",
			"claims": "",
		}

		jwtFH, err := os.Open(path)
		if err != nil {
			res["error"] = fmt.Sprintf("unable to access file: %s", err)
			results = append(results, res)
			continue
		}

		defer jwtFH.Close()

		tokenRaw, err := io.ReadAll(jwtFH)
		if err != nil {
			res["error"] = fmt.Sprintf("unable to read JWT: %s", err)
			results = append(results, res)
			continue
		}

		token, _, err := new(jwt.Parser).ParseUnverified(string(tokenRaw), jwt.MapClaims{})
		if err != nil {
			res["error"] = fmt.Sprintf("unable to parse JWT: %s", err)
			results = append(results, res)
			continue
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			res["error"] = "JWT has no claims"
			results = append(results, res)
			continue
		}

		jsonClaims, err := json.Marshal(claims)
		if err != nil {
			res["error"] = fmt.Sprintf("unable to parse claims: %s", err)
			results = append(results, res)
			continue
		}

		res["claims"] = string(jsonClaims)

		results = append(results, res)
	}

	return results, nil
}
