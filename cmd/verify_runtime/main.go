package main

import (
	"fmt"
	"os"

	"keeper/pkg/config"
)

func main() {
	cfg := config.DefaultConfig()
	fmt.Printf("Default container runtime: %s\n", cfg.ContainerRuntime)
	if cfg.ContainerRuntime != "docker" {
		fmt.Println("ERROR: default container runtime is not docker")
		os.Exit(1)
	}
	fmt.Println("SUCCESS: default runtime is docker")
}
