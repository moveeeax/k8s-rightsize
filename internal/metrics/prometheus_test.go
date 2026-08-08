package metrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeProm answers query/query_range with canned matrix/vector data based on
// the PromQL metric name embedded in the request.
func fakeProm(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(q, "container_cpu_usage_seconds_total"):
			w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[
			  {"metric":{"namespace":"prod","container":"web"},"values":[[1,"0.1"],[2,"0.12"],[3,"0.11"]]}
			]}}`))
		case strings.Contains(q, "container_memory_working_set_bytes"):
			w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[
			  {"metric":{"namespace":"prod","container":"web"},"values":[[1,"104857600"],[2,"104857600"]]}
			]}}`))
		case strings.Contains(q, `resource_requests{resource="cpu"`):
			w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[
			  {"metric":{"namespace":"prod","container":"web"},"value":[1,"1"]}
			]}}`))
		case strings.Contains(q, `resource_requests{resource="memory"`):
			w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[
			  {"metric":{"namespace":"prod","container":"web"},"value":[1,"536870912"]}
			]}}`))
		default:
			// limits: empty vector
			w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
		}
	}))
}

func TestPromSourceCollect(t *testing.T) {
	srv := fakeProm(t)
	defer srv.Close()

	src := NewPromSource(srv.URL)
	got, err := src.Collect(context.Background(), Selector{Namespace: "prod"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d containers, want 1: %+v", len(got), got)
	}
	u := got[0]
	if u.Namespace != "prod" || u.Container != "web" {
		t.Errorf("identity = %s/%s", u.Namespace, u.Container)
	}
	if len(u.CPUCores) != 3 {
		t.Errorf("cpu samples = %d, want 3", len(u.CPUCores))
	}
	if len(u.MemBytes) != 2 {
		t.Errorf("mem samples = %d, want 2", len(u.MemBytes))
	}
	if u.CurCPUReq != 1 {
		t.Errorf("CurCPUReq = %v, want 1", u.CurCPUReq)
	}
	if u.CurMemReq != 536870912 {
		t.Errorf("CurMemReq = %v, want 536870912", u.CurMemReq)
	}
}

func TestPromSourceHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	src := NewPromSource(srv.URL)
	if _, err := src.Collect(context.Background(), Selector{}); err == nil {
		t.Fatal("expected error on HTTP 500")
	}
}
