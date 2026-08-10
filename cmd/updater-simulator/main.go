package main

import (
	"fmt"
	"os"

	simulatorcmd "github.com/cyfox-labs/updates-mysoc-ai/cmd/updater-simulator/cmd"
)

var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildTime = "unknown"
)

func main() {
	if err := simulatorcmd.Execute(Version, GitCommit, BuildTime); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
