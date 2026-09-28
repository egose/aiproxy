package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestConvertRejectsFIFO(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(fmt.Sprintf("force=%t", force), func(t *testing.T) {
			dir := t.TempDir()
			src := filepath.Join(dir, "input.hcl")
			dst := filepath.Join(dir, "output.json")
			if err := os.WriteFile(src, []byte(convertTestSource), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := syscall.Mkfifo(dst, 0o600); err != nil {
				t.Fatal(err)
			}
			args := []string{dst, "--config", src}
			if force {
				args = append(args, "--force")
			}
			out, err := runConvertCommand(t, nil, args...)
			if err == nil || !strings.Contains(err.Error(), "non-regular") || strings.Contains(out, "converted ") {
				t.Fatalf("FIFO accepted: %v, %q", err, out)
			}
			info, err := os.Lstat(dst)
			if err != nil || info.Mode()&os.ModeNamedPipe == 0 {
				t.Fatalf("FIFO replaced: %v", err)
			}
			assertConvertTempsRemoved(t, dst)
		})
	}
}
