package backtest

import (
	"strings"
	"testing"
)

func TestParseGrid_CrossesRangesKeepingExitBelowEntry(t *testing.T) {
	grid, err := ParseGrid("entry=0.1:0.3:0.1,exit=-0.1:0.2:0.1")
	if err != nil {
		t.Fatal(err)
	}
	// entries 0.1, 0.2, 0.3 × exits -0.1, 0, 0.1, 0.2, keeping exit < entry
	want := []Params{
		{0.1, -0.1}, {0.1, 0},
		{0.2, -0.1}, {0.2, 0}, {0.2, 0.1},
		{0.3, -0.1}, {0.3, 0}, {0.3, 0.1}, {0.3, 0.2},
	}
	if len(grid) != len(want) {
		t.Fatalf("grid = %v, want %v", grid, want)
	}
	for i := range want {
		if grid[i] != want[i] {
			t.Errorf("grid[%d] = %v, want %v", i, grid[i], want[i])
		}
	}
}

func TestParseGrid_RangeEndpointsAreExact(t *testing.T) {
	grid, err := ParseGrid("entry=0.3,exit=-0.3:0.0:0.1")
	if err != nil {
		t.Fatal(err)
	}
	var exits []float64
	for _, p := range grid {
		exits = append(exits, p.Exit)
	}
	// Accumulating 0.1 would give -2.7755575615628914e-17 instead of 0.
	want := []float64{-0.3, -0.2, -0.1, 0}
	if len(exits) != len(want) {
		t.Fatalf("exits = %v, want %v", exits, want)
	}
	for i := range want {
		if exits[i] != want[i] {
			t.Errorf("exits[%d] = %v, want %v", i, exits[i], want[i])
		}
	}
}

func TestParseGrid_SingleValuesAndWhitespace(t *testing.T) {
	grid, err := ParseGrid(" Entry = 0.2 , exit = -0.2 ")
	if err != nil {
		t.Fatal(err)
	}
	if len(grid) != 1 || grid[0] != DefaultParams {
		t.Errorf("grid = %v, want [%v]", grid, DefaultParams)
	}
}

func TestParseGrid_Errors(t *testing.T) {
	cases := map[string]string{
		"":                                "expected entry",
		"entry=0.2":                       "both entry and exit",
		"entry=0.2,exit=-0.2,entry=0.3":   "given twice",
		"entry=0.2,stop=-0.2":             "expected entry",
		"entry=abc,exit=0":                "not a number",
		"entry=0.5:0.1:0.1,exit=0":        "start <= stop",
		"entry=0.1:0.5:0,exit=0":          "step > 0",
		"entry=0.1:0.5,exit=0":            "start:stop:step",
		"entry=1.5,exit=0":                "outside the score range",
		"entry=0.1,exit=0.1:0.5:0.1":      "no pair has exit < entry",
		"entry=-1:1:0.001,exit=-1:1:0.01": "more than the limit",
	}
	for spec, want := range cases {
		_, err := ParseGrid(spec)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("ParseGrid(%q) error = %v, want it to mention %q", spec, err, want)
		}
	}
}
