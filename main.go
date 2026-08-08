// Command k8s-rightsize suggests CPU/memory requests and limits for Kubernetes
// containers from real usage metrics. It is strictly read-only: it queries
// Prometheus (or a usage file) and prints a report — it never touches a
// workload.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/moveeeax/k8s-rightsize/internal/metrics"
	"github.com/moveeeax/k8s-rightsize/internal/report"
	"github.com/moveeeax/k8s-rightsize/internal/rightsize"
)

var version = "v0.1.0"

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "k8s-rightsize:", err)
		os.Exit(1)
	}
}

func run(args []string, out *os.File) error {
	fs := flag.NewFlagSet("k8s-rightsize", flag.ContinueOnError)
	var (
		namespace = fs.String("namespace", "", "restrict to a namespace (default: all)")
		selector  = fs.String("selector", "", "extra PromQL label matcher, e.g. app=\"web\"")
		source    = fs.String("source", "prometheus", "metrics source: prometheus|file")
		promURL   = fs.String("prometheus", envOr("PROMETHEUS_URL", "http://localhost:9090"), "Prometheus base URL")
		file      = fs.String("file", "", "usage JSON file (for --source file)")
		window    = fs.Duration("window", 24*time.Hour, "usage look-back window")
		step      = fs.Duration("step", 5*time.Minute, "sample step")
		output    = fs.String("output", "table", "output format: table|json")
		reqPct    = fs.Float64("request-percentile", 50, "usage percentile for requests")
		limPct    = fs.Float64("limit-percentile", 95, "usage percentile for limits")
		cpuCost   = fs.Float64("cpu-cost", 0, "$ per requested core per month (enables cost estimate)")
		memCost   = fs.Float64("mem-cost", 0, "$ per requested GiB per month (enables cost estimate)")
		showVer   = fs.Bool("version", false, "print version and exit")
	)
	fs.SetOutput(out)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *showVer {
		fmt.Fprintln(out, "k8s-rightsize", version)
		return nil
	}

	var src metrics.Source
	switch *source {
	case "prometheus":
		ps := metrics.NewPromSource(*promURL)
		ps.Window = *window
		ps.Step = *step
		src = ps
	case "file":
		if *file == "" {
			return fmt.Errorf("--source file requires --file")
		}
		src = metrics.FileSource{Path: *file}
	default:
		return fmt.Errorf("unknown source %q (want prometheus|file)", *source)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	usages, err := src.Collect(ctx, metrics.Selector{Namespace: *namespace, Selector: *selector})
	if err != nil {
		return err
	}
	if len(usages) == 0 {
		return fmt.Errorf("no container usage found for the given selector")
	}

	cfg := rightsize.DefaultConfig()
	cfg.RequestPercentile = *reqPct
	cfg.LimitPercentile = *limPct
	price := rightsize.Pricing{CPUCorePerMonth: *cpuCost, MemGiBPerMonth: *memCost}
	rep := rightsize.Analyze(usages, cfg, price)

	switch *output {
	case "json":
		return report.JSON(out, rep)
	case "table":
		return report.Table(out, rep)
	default:
		return fmt.Errorf("unknown output %q (want table|json)", *output)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
