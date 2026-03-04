//go:build !windows

package main

import (
	"fmt"

	"github.com/juanmferreira93/iracing-agent/internal/config"
)

func runTray(_ config.Config, _ []string) error {
	return fmt.Errorf("tray mode is only supported on Windows")
}
