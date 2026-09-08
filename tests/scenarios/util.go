package scenarios

import (
	"fmt"
	"strings"
)

// testPrint exists for lint detection of fmt.Print* calls
// each line is prepended with "test output: " to make the origin of the output obvious
func testPrintln(a ...any) {
	msg := strings.TrimSuffix(fmt.Sprintln(a...), "\n")
	for _, line := range strings.Split(msg, "\n") {
		fmt.Println("test output: " + line)
	}
}
