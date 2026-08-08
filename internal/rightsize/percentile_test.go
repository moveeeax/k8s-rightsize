package rightsize

import "testing"

func almost(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 1e-9
}

func TestPercentile(t *testing.T) {
	s := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	cases := []struct {
		p    float64
		want float64
	}{
		{0, 1},
		{100, 10},
		{50, 5.5},  // linear interp between 5 and 6
		{95, 9.55}, // rank 8.55 -> 9 + .55*(10-9)
		{25, 3.25},
	}
	for _, c := range cases {
		if got := Percentile(s, c.p); !almost(got, c.want) {
			t.Errorf("Percentile(p%v) = %v, want %v", c.p, got, c.want)
		}
	}
}

func TestPercentileEdgeCases(t *testing.T) {
	if got := Percentile(nil, 50); got != 0 {
		t.Errorf("empty = %v, want 0", got)
	}
	if got := Percentile([]float64{42}, 95); got != 42 {
		t.Errorf("single = %v, want 42", got)
	}
}

func TestPercentileDoesNotMutate(t *testing.T) {
	s := []float64{3, 1, 2}
	_ = Percentile(s, 50)
	if s[0] != 3 || s[1] != 1 || s[2] != 2 {
		t.Errorf("input mutated: %v", s)
	}
}

func TestMax(t *testing.T) {
	if got := Max([]float64{2, 9, 4}); got != 9 {
		t.Errorf("Max = %v, want 9", got)
	}
	if got := Max(nil); got != 0 {
		t.Errorf("Max(nil) = %v, want 0", got)
	}
}
