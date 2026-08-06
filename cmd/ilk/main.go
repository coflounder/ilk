// Command ilk composes the process layers an AI-native repository runs on.
package main

import (
	"os"

	"github.com/coflounder/ilk/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
