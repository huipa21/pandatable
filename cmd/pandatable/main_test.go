package main

import "testing"

func TestRun(t *testing.T) {
	for _, test := range []struct {
		args []string
		want int
	}{
		{[]string{"version"}, 0},
		{[]string{"serve"}, 1},
		{nil, 2},
		{[]string{"unknown"}, 2},
		{[]string{"version", "extra"}, 2},
	} {
		if got := run(test.args); got != test.want {
			t.Errorf("run(%v) = %d, want %d", test.args, got, test.want)
		}
	}
}
