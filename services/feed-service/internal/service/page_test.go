package service

import "testing"

// TestClampLimit covers the P1 cursor pagination contract (plan §3.3):
// 0/absent → 20, >50 → 50, <0 → 20, 1..50 unchanged.
func TestClampLimit(t *testing.T) {
	cases := []struct {
		in   int
		want int
	}{
		{in: 0, want: 20},
		{in: -1, want: 20},
		{in: 100, want: 50},
		{in: 51, want: 50},
		{in: 50, want: 50},
		{in: 20, want: 20},
		{in: 1, want: 1},
	}
	for _, c := range cases {
		if got := clampLimit(c.in); got != c.want {
			t.Errorf("clampLimit(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}
