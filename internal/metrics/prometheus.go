package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// PromSource collects usage from a Prometheus HTTP API. It runs range queries
// for CPU/memory usage and instant queries for the currently configured
// requests/limits (kube-state-metrics), then merges everything by
// (namespace, container). Everything is read-only.
type PromSource struct {
	BaseURL string
	Client  *http.Client
	Window  time.Duration // look-back window for usage, e.g. 24h
	Step    time.Duration // sample step, e.g. 5m
	Now     time.Time     // evaluation end time; zero => time.Now()
}

// NewPromSource builds a source with sane defaults (24h window, 5m step).
func NewPromSource(baseURL string) *PromSource {
	return &PromSource{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Client:  &http.Client{Timeout: 30 * time.Second},
		Window:  24 * time.Hour,
		Step:    5 * time.Minute,
	}
}

// promResponse mirrors the subset of the Prometheus API we consume.
type promResponse struct {
	Status string `json:"status"`
	Error  string `json:"error"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Value  []interface{}     `json:"value"`  // instant: [ts, "val"]
			Values [][]interface{}   `json:"values"` // range: [[ts,"val"], ...]
		} `json:"result"`
	} `json:"data"`
}

func matcher(sel Selector, extra string) string {
	parts := []string{`container!="", container!="POD"`}
	if sel.Namespace != "" {
		parts = append(parts, fmt.Sprintf(`namespace=%q`, sel.Namespace))
	}
	if sel.Selector != "" {
		parts = append(parts, sel.Selector)
	}
	if extra != "" {
		parts = append(parts, extra)
	}
	return strings.Join(parts, ", ")
}

// Collect runs the four queries and merges them.
func (p *PromSource) Collect(ctx context.Context, sel Selector) ([]ContainerUsage, error) {
	now := p.Now
	if now.IsZero() {
		now = time.Now()
	}
	m := matcher(sel, "")

	cpuQ := fmt.Sprintf(`sum by (namespace, container) (rate(container_cpu_usage_seconds_total{%s}[5m]))`, m)
	memQ := fmt.Sprintf(`sum by (namespace, container) (container_memory_working_set_bytes{%s})`, m)

	idx := map[string]*ContainerUsage{}
	get := func(ns, c string) *ContainerUsage {
		k := ns + "\x00" + c
		u := idx[k]
		if u == nil {
			u = &ContainerUsage{Namespace: ns, Container: c}
			idx[k] = u
		}
		return u
	}

	cpu, err := p.queryRange(ctx, cpuQ, now)
	if err != nil {
		return nil, fmt.Errorf("cpu usage query: %w", err)
	}
	for _, s := range cpu {
		get(s.metric["namespace"], s.metric["container"]).CPUCores = s.values
	}
	mem, err := p.queryRange(ctx, memQ, now)
	if err != nil {
		return nil, fmt.Errorf("memory usage query: %w", err)
	}
	for _, s := range mem {
		get(s.metric["namespace"], s.metric["container"]).MemBytes = s.values
	}

	// Current requests/limits from kube-state-metrics (best effort).
	reqM := matcher(sel, "")
	for _, q := range []struct {
		expr   string
		assign func(u *ContainerUsage, v float64)
	}{
		{fmt.Sprintf(`sum by (namespace, container) (kube_pod_container_resource_requests{resource="cpu", %s})`, reqM),
			func(u *ContainerUsage, v float64) { u.CurCPUReq = v }},
		{fmt.Sprintf(`sum by (namespace, container) (kube_pod_container_resource_requests{resource="memory", %s})`, reqM),
			func(u *ContainerUsage, v float64) { u.CurMemReq = v }},
		{fmt.Sprintf(`sum by (namespace, container) (kube_pod_container_resource_limits{resource="cpu", %s})`, reqM),
			func(u *ContainerUsage, v float64) { u.CurCPULim = v }},
		{fmt.Sprintf(`sum by (namespace, container) (kube_pod_container_resource_limits{resource="memory", %s})`, reqM),
			func(u *ContainerUsage, v float64) { u.CurMemLim = v }},
	} {
		vec, err := p.queryInstant(ctx, q.expr, now)
		if err != nil {
			return nil, fmt.Errorf("requests/limits query: %w", err)
		}
		for _, s := range vec {
			if len(s.values) == 0 {
				continue
			}
			q.assign(get(s.metric["namespace"], s.metric["container"]), s.values[0])
		}
	}

	out := make([]ContainerUsage, 0, len(idx))
	for _, u := range idx {
		// Only keep containers we actually observed using something.
		if len(u.CPUCores) == 0 && len(u.MemBytes) == 0 {
			continue
		}
		out = append(out, *u)
	}
	return out, nil
}

type series struct {
	metric map[string]string
	values []float64
}

func (p *PromSource) queryRange(ctx context.Context, expr string, end time.Time) ([]series, error) {
	start := end.Add(-p.Window)
	v := url.Values{}
	v.Set("query", expr)
	v.Set("start", strconv.FormatInt(start.Unix(), 10))
	v.Set("end", strconv.FormatInt(end.Unix(), 10))
	v.Set("step", strconv.Itoa(int(p.Step.Seconds())))
	body, err := p.do(ctx, "/api/v1/query_range", v)
	if err != nil {
		return nil, err
	}
	var out []series
	for _, r := range body.Data.Result {
		s := series{metric: r.Metric}
		for _, pair := range r.Values {
			if len(pair) == 2 {
				s.values = append(s.values, parseVal(pair[1]))
			}
		}
		out = append(out, s)
	}
	return out, nil
}

func (p *PromSource) queryInstant(ctx context.Context, expr string, at time.Time) ([]series, error) {
	v := url.Values{}
	v.Set("query", expr)
	v.Set("time", strconv.FormatInt(at.Unix(), 10))
	body, err := p.do(ctx, "/api/v1/query", v)
	if err != nil {
		return nil, err
	}
	var out []series
	for _, r := range body.Data.Result {
		s := series{metric: r.Metric}
		if len(r.Value) == 2 {
			s.values = append(s.values, parseVal(r.Value[1]))
		}
		out = append(out, s)
	}
	return out, nil
}

func (p *PromSource) do(ctx context.Context, path string, v url.Values) (*promResponse, error) {
	u := p.BaseURL + path + "?" + v.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prometheus %s: HTTP %d", path, resp.StatusCode)
	}
	var pr promResponse
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, fmt.Errorf("decode prometheus response: %w", err)
	}
	if pr.Status != "success" {
		return nil, fmt.Errorf("prometheus error: %s", pr.Error)
	}
	return &pr, nil
}

func parseVal(x interface{}) float64 {
	if s, ok := x.(string); ok {
		f, _ := strconv.ParseFloat(s, 64)
		return f
	}
	if f, ok := x.(float64); ok {
		return f
	}
	return 0
}
