package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/guluc3m/ventoy-updater/internal/app"
)

// main parses CLI flags and starts the ISO manager.
func main() {
	ventoy := flag.String("ventoy-dir", "", "path to the ventoy directory on the USB drive (auto-detected when omitted)")
	assumeYes := flag.Bool("yes", false, "download every discovered update and delete replaced ISOs")
	noDelete := flag.Bool("keep-old", false, "keep old ISOs after a successful update, pre-marking the keep column")
	flag.Parse()
	explicit := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "ventoy-dir" {
			explicit = true
		}
	})
	if err := app.Run(*ventoy, explicit, *assumeYes, *noDelete); err != nil {
		fmt.Fprintln(os.Stderr, "ventoy-manager:", err)
		os.Exit(1)
	}
}
