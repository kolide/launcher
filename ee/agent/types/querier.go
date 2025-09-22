package types

type InstanceStatus string

const (
	InstanceStatusNotStarted InstanceStatus = "not_started"
	InstanceStatusHealthy    InstanceStatus = "healthy"
	InstanceStatusUnhealthy  InstanceStatus = "unhealthy"
)

// InstanceQuerier is implemented by pkg/osquery/runtime/runner.go
type InstanceQuerier interface {
	InstanceStatuses() map[string]InstanceStatus
}
