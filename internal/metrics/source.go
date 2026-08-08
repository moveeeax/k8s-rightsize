// Package metrics defines the usage-sample model and the source interface that
// feeds the right-sizing engine. A Source is anything that can produce
// per-container CPU/memory usage samples plus the currently configured
// requests/limits — read-only, always.
package metrics

import "context"

// ContainerUsage holds the raw material for a single (namespace, container)
// right-sizing decision: the observed usage samples over the query window and
// the resources currently requested/limited on the workload.
//
// CPU values are in cores, memory values in bytes. A zero "current" value means
// "not set / unknown" and is reported as such rather than treated as 0.
type ContainerUsage struct {
	Namespace string    `json:"namespace"`
	Container string    `json:"container"`
	CPUCores  []float64 `json:"cpuCores"` // usage samples, cores
	MemBytes  []float64 `json:"memBytes"` // usage samples, bytes

	CurCPUReq float64 `json:"curCpuReq"` // cores, 0 = unset
	CurMemReq float64 `json:"curMemReq"` // bytes, 0 = unset
	CurCPULim float64 `json:"curCpuLim"` // cores, 0 = unset
	CurMemLim float64 `json:"curMemLim"` // bytes, 0 = unset
}

// Selector scopes a collection to a namespace and/or a Prometheus-style label
// matcher. An empty field means "no filter on this dimension".
type Selector struct {
	Namespace string // exact namespace, "" = all
	Selector  string // extra PromQL label matcher body, e.g. `app="web"`
}

// Source produces usage records. Implementations must be read-only.
type Source interface {
	Collect(ctx context.Context, sel Selector) ([]ContainerUsage, error)
}
