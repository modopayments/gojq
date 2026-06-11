// gojq - Go implementation of jq
package main

import (
	"os"

	"github.com/modopayments/gojq/cli"
)

func main() {
	os.Exit(cli.Run())
}
