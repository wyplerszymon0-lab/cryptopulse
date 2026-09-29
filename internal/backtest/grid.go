package backtest

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// MaxGridSize bounds a custom grid: every point is backtested on every
// training window, so a few thousand points would take minutes per coin.
const MaxGridSize = 500

// ParseGrid builds a threshold grid from a spec such as
//
//	entry=0.1:0.5:0.1,exit=-0.5:0.1:0.1
//
// Each key takes an inclusive start:stop:step range or a single value. Only
// pairs with exit < entry are kept, as in DefaultGrid. Scores live in [-1, 1],
// so thresholds outside that range could never trigger and are rejected.
func ParseGrid(spec string) ([]Params, error) {
	values := map[string][]float64{}
	for _, part := range strings.Split(spec, ",") {
		key, rng, ok := strings.Cut(strings.TrimSpace(part), "=")
		key = strings.ToLower(strings.TrimSpace(key))
		if !ok || (key != "entry" && key != "exit") {
			return nil, fmt.Errorf("grid: expected entry=... and exit=..., got %q", part)
		}
		if _, dup := values[key]; dup {
			return nil, fmt.Errorf("grid: %s given twice", key)
		}
		vs, err := parseRange(rng)
		if err != nil {
			return nil, fmt.Errorf("grid: %s: %w", key, err)
		}
		values[key] = vs
	}
	if values["entry"] == nil || values["exit"] == nil {
		return nil, fmt.Errorf("grid: both entry and exit are required, e.g. entry=0.1:0.5:0.1,exit=-0.5:0.1:0.1")
	}

	var grid []Params
	for _, entry := range values["entry"] {
		for _, exit := range values["exit"] {
			if exit < entry {
				grid = append(grid, Params{Entry: entry, Exit: exit})
			}
		}
	}
	switch {
	case len(grid) == 0:
		return nil, fmt.Errorf("grid: no pair has exit < entry")
	case len(grid) > MaxGridSize:
		return nil, fmt.Errorf("grid: %d pairs is more than the limit of %d; use a coarser step", len(grid), MaxGridSize)
	}
	return grid, nil
}

// parseRange reads "start:stop:step" (stop inclusive) or a single number.
func parseRange(s string) ([]float64, error) {
	fields := strings.Split(strings.TrimSpace(s), ":")
	nums := make([]float64, len(fields))
	for i, f := range fields {
		v, err := strconv.ParseFloat(strings.TrimSpace(f), 64)
		if err != nil {
			return nil, fmt.Errorf("%q is not a number", f)
		}
		nums[i] = v
	}

	var out []float64
	switch len(nums) {
	case 1:
		out = nums
	case 3:
		start, stop, step := nums[0], nums[1], nums[2]
		if step <= 0 || start > stop {
			return nil, fmt.Errorf("range %q needs start <= stop and step > 0", s)
		}
		// Count steps instead of accumulating, so 0.1 + 0.1 + 0.1 doesn't drift past stop.
		n := int(math.Floor((stop-start)/step + 1e-9))
		for i := 0; i <= n; i++ {
			out = append(out, math.Round((start+float64(i)*step)*1e9)/1e9)
		}
	default:
		return nil, fmt.Errorf("%q: use start:stop:step or a single value", s)
	}
	for _, v := range out {
		if v < -1 || v > 1 {
			return nil, fmt.Errorf("%g is outside the score range [-1, 1]", v)
		}
	}
	return out, nil
}
