// apigen emits the Hosted API contract from internal/contract.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"portico.local/hosted/internal/contract"
)

func main() {
	root := flag.String("root", "../..", "Repository root")
	flag.Parse()
	files, err := contract.Files()
	if err != nil {
		fail(err)
	}
	for name, raw := range files {
		target := filepath.Join(*root, name)
		if err = os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			fail(err)
		}
		if err = os.WriteFile(target, raw, 0644); err != nil {
			fail(err)
		}
	}
}
func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
