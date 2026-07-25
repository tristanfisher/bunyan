package main

import (
	"bunyan/tests/scenarios"
	"testing"
)

func Test_scenarios(t *testing.T) {
	// last call must be blocking

	//err := scenarios.Simple()
	//if err != nil {
	//	t.Error("simple test: ", err)
	//}

	err := scenarios.Morning()
	if err != nil {
		t.Error("morning test: ", err)
	}

	//scenarios.HighConcurrency(5 * time.Second)

}
