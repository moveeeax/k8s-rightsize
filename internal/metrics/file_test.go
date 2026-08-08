package metrics

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFileSource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "usage.json")
	data := `[
	  {"namespace":"prod","container":"web","cpuCores":[0.1,0.2],"memBytes":[100,200],"curCpuReq":1.0},
	  {"namespace":"dev","container":"job","cpuCores":[0.3],"memBytes":[50]}
	]`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	src := FileSource{Path: path}
	all, err := src.Collect(context.Background(), Selector{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("got %d records, want 2", len(all))
	}

	only, err := src.Collect(context.Background(), Selector{Namespace: "prod"})
	if err != nil {
		t.Fatal(err)
	}
	if len(only) != 1 || only[0].Container != "web" {
		t.Fatalf("namespace filter failed: %+v", only)
	}
	if only[0].CurCPUReq != 1.0 {
		t.Errorf("CurCPUReq = %v, want 1.0", only[0].CurCPUReq)
	}
}

func TestFileSourceMissing(t *testing.T) {
	src := FileSource{Path: "/no/such/file.json"}
	if _, err := src.Collect(context.Background(), Selector{}); err == nil {
		t.Fatal("expected error for missing file")
	}
}
