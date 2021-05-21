package debug

import (
	"fmt"
	"os"

	"github.com/alecthomas/hcl"
)

// Flags set from the HCL formatted HERMIT_DEBUG envar.
var Flags struct {
	KeepLogs        bool `hcl:"keeplogs,optional" help:"Don't clear logs after executing.'"`
	AlwaysCheckSelf bool `hcl:"alwayscheckself,optional" help:"Always check if Hermit itself needs updating."`
	FailHTTP        bool `hcl:"failhttp,optional" help:"Always fail HTTP requests."`
}

func init() {
	envar := os.Getenv("HERMIT_DEBUG")
	err := hcl.Unmarshal([]byte(envar), &Flags, hcl.BareBooleanAttributes(true))
	if err != nil {
		baseErr := err
		schema, err := hcl.Schema(&Flags)
		if err != nil {
			panic(err)
		}
		schemaBytes, err := hcl.MarshalAST(schema)
		if err != nil {
			panic(err)
		}
		fmt.Fprintf(os.Stderr, "Invalid HERMIT_DEBUG=%q: %s\n\nSchema:\n\n%s\n", envar, baseErr, string(schemaBytes))
		os.Exit(1)
	}
}
