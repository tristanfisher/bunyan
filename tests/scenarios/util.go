package scenarios

import (
	"fmt"
)

// testPrint exists for lint detection of fmt.Print* calls
func testPrintln(a ...any) {
	fmt.Println("test output: ", a)
}
