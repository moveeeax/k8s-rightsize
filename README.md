# k8s-rightsize

> Stop guessing requests/limits — base them on what pods actually use.

**Status:** 🚧 In development

## Overview

Suggest CPU/memory requests and limits from real usage metrics (mini right-sizing report).

## Features

- Pulls usage from metrics-server/Prometheus
- Recommends requests (p50) and limits (p95) per container
- Flags over- and under-provisioned workloads
- Estimated cluster resource/cost savings
- Namespace and label selectors

## Stack

Go + client-go + Prometheus API.

## Usage

```bash
k8s-rightsize --namespace prod --source prometheus
```

## License

MIT
