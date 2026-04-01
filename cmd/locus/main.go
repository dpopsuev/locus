package main

import (
	"os"
	"runtime/debug"

	"github.com/dpopsuev/locus/internal/cli"
)

// Version is set by -ldflags at build time.
var Version = "dev"

func main() {
	v := Version
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 7 {
				v += " (" + s.Value[:7] + ")"
				break
			}
		}
	}
	if err := cli.Execute(v); err != nil {
		os.Exit(1)
	}
}
