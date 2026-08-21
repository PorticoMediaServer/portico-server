package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/PorticoMediaServer/portico-server/internal/mediatoolchain"
)

func main() {
	root := flag.String("root", "", "toolchain bundle root")
	target := flag.String("target", "", "release target")
	requirements := flag.String("requirements", "", "requirements manifest")
	flag.Parse()
	if *root == "" || *target == "" || *requirements == "" {
		fmt.Fprintln(os.Stderr, "root, target, and requirements are required")
		os.Exit(2)
	}
	manifest, err := mediatoolchain.ValidateBundle(*root, *target, *requirements)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("validated media toolchain %s for %s\n", manifest.BuildID, manifest.Target)
}
