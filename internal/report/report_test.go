package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/moveeeax/k8s-rightsize/internal/rightsize"
)

func sample() rightsize.Report {
	return rightsize.Report{
		Recommendations: []rightsize.Recommendation{{
			Namespace: "prod", Container: "web", Status: rightsize.StatusOver,
			CPURequest: 0.1, CPULimit: 0.12, MemRequest: 100e6,
			CurCPUReq: 1.0, CurMemReq: 500e6, CPUReqDelta: 0.9, MemReqDelta: 400e6,
			Samples: 5,
		}},
		CPUSavedCores: 0.9, MemSavedBytes: 400e6,
		CPUCostSaved: 18, MemCostSaved: 2,
	}
}

func TestTable(t *testing.T) {
	var buf bytes.Buffer
	if err := Table(&buf, sample()); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"NAMESPACE", "web", "over", "Reclaimable", "monthly savings"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q\n%s", want, out)
		}
	}
	if !strings.Contains(out, "100m") { // 0.1 cores rendered as 100m
		t.Errorf("expected millicore formatting in\n%s", out)
	}
}

func TestJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := JSON(&buf, sample()); err != nil {
		t.Fatal(err)
	}
	var back rightsize.Report
	if err := json.Unmarshal(buf.Bytes(), &back); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if len(back.Recommendations) != 1 || back.Recommendations[0].Container != "web" {
		t.Errorf("round-trip mismatch: %+v", back)
	}
}

func TestMemFormat(t *testing.T) {
	cases := map[float64]string{0: "-", 1024: "1Ki", 104857600: "100Mi", 1073741824: "1Gi"}
	for in, want := range cases {
		if got := mem(in); got != want {
			t.Errorf("mem(%v) = %q, want %q", in, got, want)
		}
	}
}
