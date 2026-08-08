package rightsize

import (
	"math"
	"sort"
)

// Percentile returns the p-th percentile (0..100) of the samples using linear
// interpolation between closest ranks — the same method Prometheus'
// quantile() uses, so recommendations line up with what operators see in
// Grafana. The input is not mutated.
func Percentile(samples []float64, p float64) float64 {
	n := len(samples)
	if n == 0 {
		return 0
	}
	if n == 1 {
		return samples[0]
	}
	sorted := make([]float64, n)
	copy(sorted, samples)
	sort.Float64s(sorted)

	if p <= 0 {
		return sorted[0]
	}
	if p >= 100 {
		return sorted[n-1]
	}
	rank := (p / 100) * float64(n-1)
	lo := int(math.Floor(rank))
	hi := int(math.Ceil(rank))
	if lo == hi {
		return sorted[lo]
	}
	frac := rank - float64(lo)
	return sorted[lo] + frac*(sorted[hi]-sorted[lo])
}

// Max returns the largest sample, or 0 for an empty slice.
func Max(samples []float64) float64 {
	m := 0.0
	for i, v := range samples {
		if i == 0 || v > m {
			m = v
		}
	}
	return m
}
