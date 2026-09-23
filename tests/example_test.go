package main

import (
	"bunyan/tests/scenarios"
	"testing"
	"time"
)

func Test_scenarios(t *testing.T) {
	// last call must be blocking if trying to parallelize
	err := scenarios.Simple()
	if err != nil {
		t.Error("simple test: ", err)
	}

	z, err := scenarios.Morning()
	if err != nil {
		t.Error("morning test: ", err)
	}
	_, _ = z.Report()

	scenarios.HighConcurrency(5 * time.Second)
}
