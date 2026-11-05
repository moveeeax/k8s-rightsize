# k8s-rightsize

[![ci](https://github.com/moveeeax/k8s-rightsize/actions/workflows/ci.yml/badge.svg)](https://github.com/moveeeax/k8s-rightsize/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/go-1.22-00ADD8?logo=go)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

Stop guessing CPU/memory requests and limits — base them on what your pods
**actually use**. `k8s-rightsize` reads real usage from Prometheus, recommends
requests (p50) and limits (p95) per container, flags the workloads wasting the
most capacity, and estimates what you'd reclaim. It is strictly **read-only** —
it never touches a workload.

## What it does

- Pulls per-container CPU/memory usage from Prometheus over a look-back window.
- Recommends **requests = p50**, **limits = p95 × headroom** of observed usage.
- Compares against the currently configured requests (from kube-state-metrics)
  and labels each container `over` / `under` / `ok` / `unknown`.
- Ranks the biggest waste offenders first and totals the reclaimable
  CPU / memory (with an optional `$`/month estimate).
- Prints a table or machine-readable JSON.

## Install

```sh
go install github.com/moveeeax/k8s-rightsize@latest
```

or build from source:

```sh
git clone https://github.com/moveeeax/k8s-rightsize
cd k8s-rightsize && go build -o k8s-rightsize .
```

## Usage

Against a live Prometheus:

```sh
k8s-rightsize --namespace prod --source prometheus --prometheus http://prometheus:9090
```

Offline, against the bundled example fixture (no cluster required):

```sh
$ k8s-rightsize --source file --file examples/usage.json
NAMESPACE  CONTAINER  STATUS   CPU req (cur→rec)  MEM req (cur→rec)  CPU save
prod       web        over     1.00 → 105m        512Mi → 100Mi      895m
staging    cache      over     500m → 20m         1Gi → 512Mi        480m
dev        sandbox    unknown  - → 180m           - → 64Mi           -
prod       worker     under    100m → 505m        256Mi → 271Mi      -

Reclaimable: 1.38 CPU, 924Mi memory across 4 container(s)
```

JSON output with a cost estimate:

```sh
k8s-rightsize --source file --file examples/usage.json --output json \
  --cpu-cost 20 --mem-cost 5
```

### Flags

| flag | default | meaning |
|------|---------|---------|
| `--source` | `prometheus` | `prometheus` or `file` |
| `--prometheus` | `$PROMETHEUS_URL` / `http://localhost:9090` | Prometheus base URL |
| `--file` | | usage JSON (for `--source file`) |
| `--namespace` | all | restrict to one namespace |
| `--selector` | | extra PromQL label matcher, e.g. `app="web"` |
| `--window` | `24h` | usage look-back window |
| `--step` | `5m` | sample step |
| `--request-percentile` | `50` | percentile used for requests |
| `--limit-percentile` | `95` | percentile used for limits |
| `--cpu-cost` / `--mem-cost` | `0` | $/core-month, $/GiB-month (enables cost estimate) |
| `--output` | `table` | `table` or `json` |

## How it works

The core is a pure function: given usage samples per container it computes
percentiles (linear interpolation, matching Prometheus' `quantile()`), derives
requests/limits, and classifies the container. The Prometheus source runs two
range queries (`rate(container_cpu_usage_seconds_total[5m])`,
`container_memory_working_set_bytes`) for usage and instant
`kube_pod_container_resource_requests/limits` queries for the current config,
then merges everything by `(namespace, container)`.

Under-provisioning wins over over-provisioning: a container whose p95 usage
already exceeds its request is at risk of throttling or OOM, so that signal is
surfaced first.

## Development

```sh
go test ./...   # unit tests, no cluster needed
go vet ./...
```

The `--source file` path uses the exact same engine as the live path, so the
example fixture doubles as a deterministic test corpus.

## License

MIT — see [LICENSE](LICENSE).

---

**Archived on 2026-11-05.** merge into k8s-resource-recommender: same job (Prometheus usage to requests/limits), but this repo keys usage by (namespace, container) so same-named containers in different workloads collapse into one row, it has no workload mapping or patch output, and it pulls raw samples for the whole window to the client; port its comparison and cost pieces, then archive. The scope moves to [k8s-resource-recommender](https://github.com/moveeeax/k8s-resource-recommender/issues/2).
