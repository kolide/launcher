package service

import "context"

type Query struct {
	ID    string
	Query string
}

type Result struct {
	ID     string
	Status int
	Rows   []map[string]string
}

type LogType int

const (
	LogTypeResult LogType = iota
	LogTypeStatus
	LogTypeAgent
)

type Log struct {
	Data string
}

type KolideService interface {
	RequestEnrollment(ctx context.Context, enrollSecret, hostIdentifer string) (string, bool, error)
	RequestConfig(ctx context.Context, nodeKey, version string) (string, bool, error)
	RequestQueries(ctx context.Context, nodeKey, version string) ([]Query, bool, error)
	PublishLogs(ctx context.Context, nodeKey, version string, logType LogType, logs []Log) (string, string, bool, error)
	PublishResults(ctx context.Context, nodeKey string, results []Result) (string, string, bool, error)
}
