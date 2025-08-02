// Command sysvargen generates system variable registration code from struct tags.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/apstndb/spanner-mycli/tools/sysvargen"
)

func main() {
	var (
		input   = flag.String("input", "", "Input Go file to parse")
		output  = flag.String("output", "", "Output file (default: stdout)")
		pkgName = flag.String("package", "main", "Package name for generated code")
	)
	flag.Parse()

	if *input == "" {
		flag.Usage()
		os.Exit(1)
	}

	gen := sysvargen.NewGenerator(*pkgName)

	if err := gen.ParseFile(*input); err != nil {
		log.Fatalf("Failed to parse file: %v", err)
	}

	code, err := gen.Generate()
	if err != nil {
		log.Fatalf("Failed to generate code: %v", err)
	}

	if *output != "" {
		if err := os.WriteFile(*output, code, 0o644); err != nil {
			log.Fatalf("Failed to write output: %v", err)
		}
		fmt.Printf("Generated %s\n", *output)
	} else {
		fmt.Print(string(code))
	}
}
