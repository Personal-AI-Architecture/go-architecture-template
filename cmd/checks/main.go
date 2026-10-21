package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Personal-AI-Architecture/go-architecture-template/internal/checks"
)

func main() {
	sourceCheckout := flag.String("source-checkout", "", "local PAA Git checkout")
	formatOnly := flag.Bool("format-only", false, "run only the gofmt check")
	flag.Parse()

	root, err := os.Getwd()
	if err != nil {
		fmt.Println("unable to determine working directory:", err)
		os.Exit(1)
	}
	var errors []string
	if *formatOnly {
		errors = checks.CheckFormat(root)
	} else {
		errors = checks.Run(root, *sourceCheckout)
	}
	if len(errors) > 0 {
		fmt.Println("Go PAA checks failed:")
		for _, checkError := range errors {
			fmt.Println("-", checkError)
		}
		os.Exit(1)
	}
	fmt.Println("Go PAA checks passed.")
}
