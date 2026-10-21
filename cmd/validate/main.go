package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	sourceCheckout := flag.String("source-checkout", "", "local PAA Git checkout")
	sharedValidator := flag.String("shared-validator", "", "path to paa-validate.js")
	flag.Parse()

	commands := [][]string{
		{"run", "./cmd/checks"},
		{"vet", "./..."},
		{"test", "./..."},
		{"run", "./cmd/acceptance"},
	}
	if *sourceCheckout != "" {
		commands[0] = append(commands[0], "--source-checkout", *sourceCheckout)
	}
	for _, command := range commands {
		if !run("go", command...) {
			os.Exit(1)
		}
	}
	if *sharedValidator != "" {
		validator, err := filepath.Abs(*sharedValidator)
		if err != nil {
			fmt.Println("unable to resolve shared validator:", err)
			os.Exit(1)
		}
		args := []string{validator, "--target", "paa.target.json"}
		if *sourceCheckout != "" {
			args = append(args, "--source-checkout", *sourceCheckout)
		}
		if !run("node", args...) {
			os.Exit(1)
		}
	}
	fmt.Println("Go PAA template validation passed.")
}

func run(program string, args ...string) bool {
	fmt.Printf("\n[%s %v]\n", program, args)
	command := exec.Command(program, args...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run() == nil
}
