package rightsize

import (
	"testing"

	"github.com/moveeeax/k8s-rightsize/internal/metrics"
)

func usages() []metrics.ContainerUsage {
	return []metrics.ContainerUsage{
		{ // over-provisioned: requests 1 core but uses ~0.1
			Namespace: "prod", Container: "web",
			CPUCores:  []float64{0.1, 0.1, 0.1, 0.1, 0.1},
			MemBytes:  []float64{100e6, 100e6, 100e6, 100e6, 100e6},
			CurCPUReq: 1.0, CurMemReq: 500e6,
		},
		{ // under-provisioned: requests 0.1 core but uses ~0.5
			Namespace: "prod", Container: "worker",
			CPUCores:  []float64{0.4, 0.45, 0.5, 0.5, 0.5},
			MemBytes:  []float64{200e6, 210e6, 220e6, 220e6, 230e6},
			CurCPUReq: 0.1, CurMemReq: 400e6,
		},
		{ // no current requests set
			Namespace: "dev", Container: "job",
			CPUCores: []float64{0.2, 0.2, 0.2},
			MemBytes: []float64{50e6, 50e6, 50e6},
		},
	}
}

func find(rep Report, ns, c string) *Recommendation {
	for i := range rep.Recommendations {
		if rep.Recommendations[i].Namespace == ns && rep.Recommendations[i].Container == c {
			return &rep.Recommendations[i]
		}
	}
	return nil
}

func TestAnalyzeStatuses(t *testing.T) {
	rep := Analyze(usages(), DefaultConfig(), Pricing{})

	if r := find(rep, "prod", "web"); r == nil || r.Status != StatusOver {
		t.Fatalf("web status = %v, want over", statusOf(r))
	}
	if r := find(rep, "prod", "worker"); r == nil || r.Status != StatusUnder {
		t.Fatalf("worker status = %v, want under", statusOf(find(rep, "prod", "worker")))
	}
	if r := find(rep, "dev", "job"); r == nil || r.Status != StatusUnknown {
		t.Fatalf("job status = %v, want unknown", statusOf(find(rep, "dev", "job")))
	}
}

func statusOf(r *Recommendation) Status {
	if r == nil {
		return "nil"
	}
	return r.Status
}

func TestAnalyzeRecommendationValues(t *testing.T) {
	rep := Analyze(usages(), DefaultConfig(), Pricing{})
	web := find(rep, "prod", "web")
	if !almost(web.CPURequest, 0.1) {
		t.Errorf("web CPU request = %v, want 0.1 (p50)", web.CPURequest)
	}
	// limit = p95(0.1) * 1.2 headroom
	if !almost(web.CPULimit, 0.12) {
		t.Errorf("web CPU limit = %v, want 0.12", web.CPULimit)
	}
	if !almost(web.CPUReqDelta, 0.9) {
		t.Errorf("web CPU delta = %v, want 0.9", web.CPUReqDelta)
	}
}

func TestAnalyzeSavingsAndSort(t *testing.T) {
	rep := Analyze(usages(), DefaultConfig(), Pricing{CPUCorePerMonth: 20, MemGiBPerMonth: 5})

	// Only the over-provisioned web container reclaims CPU (0.9 cores).
	if !almost(rep.CPUSavedCores, 0.9) {
		t.Errorf("CPUSavedCores = %v, want 0.9", rep.CPUSavedCores)
	}
	// web mem: 500e6-100e6=400e6, plus worker mem-over: 400e6-220e6=180e6.
	if !almost(rep.MemSavedBytes, 580e6) {
		t.Errorf("MemSavedBytes = %v, want 5.8e8", rep.MemSavedBytes)
	}
	// Biggest offender sorts first.
	if rep.Recommendations[0].Container != "web" {
		t.Errorf("first row = %q, want web", rep.Recommendations[0].Container)
	}
	if rep.CPUCostSaved <= 0 || rep.MemCostSaved <= 0 {
		t.Errorf("cost estimate not populated: cpu=%v mem=%v", rep.CPUCostSaved, rep.MemCostSaved)
	}
}

func TestLowConfidence(t *testing.T) {
	u := []metrics.ContainerUsage{{
		Namespace: "prod", Container: "sparse",
		CPUCores: []float64{0.3}, MemBytes: []float64{10e6},
		CurCPUReq: 0.5,
	}}
	rep := Analyze(u, DefaultConfig(), Pricing{})
	if !rep.Recommendations[0].LowConfidence {
		t.Errorf("expected low-confidence flag for single-sample container")
	}
}
