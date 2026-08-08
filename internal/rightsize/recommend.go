// Package rightsize turns raw usage samples into per-container
// request/limit recommendations and flags the workloads wasting the most
// resources. It never talks to a cluster; it only crunches numbers.
package rightsize

import (
	"sort"

	"github.com/moveeeax/k8s-rightsize/internal/metrics"
)

// Status classifies a container against its current requests.
type Status string

const (
	StatusOK      Status = "ok"      // request roughly matches usage
	StatusOver    Status = "over"    // over-provisioned: requesting far more than used
	StatusUnder   Status = "under"   // under-provisioned: usage exceeds request (throttle/OOM risk)
	StatusUnknown Status = "unknown" // no current request set, nothing to compare
)

// Config tunes the recommendation. Zero value is not valid; use DefaultConfig.
type Config struct {
	RequestPercentile float64 // usage percentile for requests (typ. 50)
	LimitPercentile   float64 // usage percentile for limits (typ. 95)
	LimitHeadroom     float64 // multiplier applied to the limit percentile (typ. 1.2)
	OverFactor        float64 // request > rec*OverFactor => over-provisioned (typ. 1.3)
	MinSamples        int     // fewer samples => flagged low-confidence
}

// DefaultConfig is the p50-request / p95-limit policy from the spec, with a bit
// of headroom on limits so a normal spike does not get OOM-killed.
func DefaultConfig() Config {
	return Config{
		RequestPercentile: 50,
		LimitPercentile:   95,
		LimitHeadroom:     1.2,
		OverFactor:        1.3,
		MinSamples:        3,
	}
}

// Recommendation is the per-container verdict.
type Recommendation struct {
	Namespace string `json:"namespace"`
	Container string `json:"container"`

	CPURequest float64 `json:"cpuRequestCores"`
	CPULimit   float64 `json:"cpuLimitCores"`
	MemRequest float64 `json:"memRequestBytes"`
	MemLimit   float64 `json:"memLimitBytes"`

	CurCPUReq float64 `json:"curCpuRequestCores"`
	CurMemReq float64 `json:"curMemRequestBytes"`

	// Deltas are current-minus-recommended requests: positive = reclaimable.
	CPUReqDelta float64 `json:"cpuRequestDeltaCores"`
	MemReqDelta float64 `json:"memRequestDeltaBytes"`

	Status        Status `json:"status"`
	LowConfidence bool   `json:"lowConfidence"`
	Samples       int    `json:"samples"`
}

// Report is the full result set plus cluster-wide savings totals.
type Report struct {
	Recommendations []Recommendation `json:"recommendations"`
	CPUSavedCores   float64          `json:"cpuSavedCores"`
	MemSavedBytes   float64          `json:"memSavedBytes"`
	CPUCostSaved    float64          `json:"cpuCostSaved,omitempty"`
	MemCostSaved    float64          `json:"memCostSaved,omitempty"`
}

// Pricing converts reclaimed capacity into a monthly cost estimate.
type Pricing struct {
	CPUCorePerMonth float64 // $ per requested core per month
	MemGiBPerMonth  float64 // $ per requested GiB per month
}

const gib = 1024 * 1024 * 1024

// Analyze produces a Report from usage records. It is deterministic and pure.
func Analyze(usages []metrics.ContainerUsage, cfg Config, price Pricing) Report {
	rep := Report{}
	for _, u := range usages {
		r := recommendOne(u, cfg)
		rep.Recommendations = append(rep.Recommendations, r)
		if r.CPUReqDelta > 0 {
			rep.CPUSavedCores += r.CPUReqDelta
		}
		if r.MemReqDelta > 0 {
			rep.MemSavedBytes += r.MemReqDelta
		}
	}
	// Biggest offenders first: rank by reclaimable CPU, then memory.
	sort.SliceStable(rep.Recommendations, func(i, j int) bool {
		a, b := rep.Recommendations[i], rep.Recommendations[j]
		if a.CPUReqDelta != b.CPUReqDelta {
			return a.CPUReqDelta > b.CPUReqDelta
		}
		return a.MemReqDelta > b.MemReqDelta
	})
	if price.CPUCorePerMonth > 0 {
		rep.CPUCostSaved = rep.CPUSavedCores * price.CPUCorePerMonth
	}
	if price.MemGiBPerMonth > 0 {
		rep.MemCostSaved = (rep.MemSavedBytes / gib) * price.MemGiBPerMonth
	}
	return rep
}

func recommendOne(u metrics.ContainerUsage, cfg Config) Recommendation {
	cpuReq := Percentile(u.CPUCores, cfg.RequestPercentile)
	cpuLim := Percentile(u.CPUCores, cfg.LimitPercentile) * cfg.LimitHeadroom
	memReq := Percentile(u.MemBytes, cfg.RequestPercentile)
	memLim := Percentile(u.MemBytes, cfg.LimitPercentile) * cfg.LimitHeadroom

	samples := len(u.CPUCores)
	if len(u.MemBytes) > samples {
		samples = len(u.MemBytes)
	}

	r := Recommendation{
		Namespace:  u.Namespace,
		Container:  u.Container,
		CPURequest: cpuReq,
		CPULimit:   cpuLim,
		MemRequest: memReq,
		MemLimit:   memLim,
		CurCPUReq:  u.CurCPUReq,
		CurMemReq:  u.CurMemReq,
		Samples:    samples,
	}
	if u.CurCPUReq > 0 {
		r.CPUReqDelta = u.CurCPUReq - cpuReq
	}
	if u.CurMemReq > 0 {
		r.MemReqDelta = u.CurMemReq - memReq
	}
	r.LowConfidence = samples < cfg.MinSamples
	r.Status = classify(u, cpuReq, memReq, cfg)
	return r
}

// classify decides over/under/ok. Under-provisioning wins over over-provisioning:
// an under-provisioned container risks throttling or OOM and is the more urgent
// signal, so we surface it even if another resource looks slack.
func classify(u metrics.ContainerUsage, cpuRec, memRec float64, cfg Config) Status {
	haveCur := u.CurCPUReq > 0 || u.CurMemReq > 0
	if !haveCur {
		return StatusUnknown
	}
	cpuP95 := Percentile(u.CPUCores, cfg.LimitPercentile)
	memP95 := Percentile(u.MemBytes, cfg.LimitPercentile)

	under := (u.CurCPUReq > 0 && cpuP95 > u.CurCPUReq) ||
		(u.CurMemReq > 0 && memP95 > u.CurMemReq)
	if under {
		return StatusUnder
	}
	over := (u.CurCPUReq > 0 && cpuRec > 0 && u.CurCPUReq > cpuRec*cfg.OverFactor) ||
		(u.CurMemReq > 0 && memRec > 0 && u.CurMemReq > memRec*cfg.OverFactor)
	if over {
		return StatusOver
	}
	return StatusOK
}
