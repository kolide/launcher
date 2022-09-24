package dataflattentable

import (
	"bytes"
	"fmt"
	"io"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/dataflatten"
	"github.com/osquery/osquery-go/plugin/table"
)

type flattenBytesInt interface {
	FlattenBytes([]byte, ...dataflatten.FlattenOpts) ([]dataflatten.Row, error)
}

type parserInt interface {
	Parse(io.Reader) (any, error)
}

func NewExecAndParseTable(
	logger log.Logger,
	tableName string,
	parser parserInt,
	execArgs []string,
	opts ...ExecTableOpt,

) *table.Plugin {
	t := &Table{
		logger:           level.NewFilter(log.With(logger, "table", tableName), level.AllowInfo()),
		tableName:        tableName,
		flattenBytesFunc: flattenerFromParser(parser).FlattenBytes,
		execArgs:         execArgs,
	}

	for _, opt := range opts {
		opt(t)
	}

	return table.NewPlugin(t.tableName, Columns(), t.generateExec)
}

type parserFlattener struct {
	parser parserInt
}

func flattenerFromParser(parser parserInt) flattenBytesInt {
	return &parserFlattener{parser: parser}
}

func (p *parserFlattener) FlattenBytes(raw []byte, flattenOpts ...dataflatten.FlattenOpts) ([]dataflatten.Row, error) {
	data, err := p.parser.Parse(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("error parsing data: %w", err)
	}

	return dataflatten.Flatten(data, flattenOpts...)
}
