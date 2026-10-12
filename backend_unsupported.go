//go:build (!linux && !windows) || (!amd64 && !arm64)

package main

import "fmt"

func run(o options) error { return fmt.Errorf("suporte a Linux e Windows de 64 bits (amd64/arm64)") }
