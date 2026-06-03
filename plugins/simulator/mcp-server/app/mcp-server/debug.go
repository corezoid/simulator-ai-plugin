package mcpserver

import (
	"log"
	"os"
)

// debugVerbose enables DEBUG-level logging. These logs fire on every request
// and can include request bodies / query params (potentially sensitive), so
// they are off unless SIMULATOR_DEBUG is set to a non-empty value.
var debugVerbose = os.Getenv("SIMULATOR_DEBUG") != ""

// debugf logs only when SIMULATOR_DEBUG is set. Same signature as log.Printf.
func debugf(format string, args ...interface{}) {
	if debugVerbose {
		log.Printf(format, args...)
	}
}
