package util

import (
	"crypto/rand"
	"fmt"
)

const EmptyUUID = "0000-0000-0000-0000-0000"

// UUID4 creates a version 4 UUID using the standard library (rfc4122 section-4.4)
func UUID4() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	// as of go1.25.2, an err in rand.Read() always panics.
	// we should not be here, but defensively code anyway
	if err != nil {
		return EmptyUUID
	}
	// clear high 4 bits, set low bits on.
	// set 0x40 (version 4 gets the value 4)
	b[6] = (b[6] & 0x0f) | 0x40
	// set variant per RFC 4122
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
