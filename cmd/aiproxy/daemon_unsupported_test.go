//go:build !linux

package main

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestDaemonLifecycleUnsupportedPlatform(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.hcl")
	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)

	checks := map[string]error{
		"spawn":   spawnDaemon(cmd, cfgPath),
		"stop":    stopServer(cfgPath, &out),
		"status":  statusServer(cfgPath, &out),
		"restart": restartServer(cmd, cfgPath),
	}
	for name, err := range checks {
		if !errors.Is(err, errDaemonLifecycleUnsupported) {
			t.Fatalf("%s error = %v, want unsupported-platform error", name, err)
		}
	}
}
