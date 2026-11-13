//go:build !unix && !windows

package config

import (
	"fmt"
	"os"
)

func openTLSProfileFile(string) (*os.File, error) {
	return nil, fmt.Errorf("nonblocking profile file access is unavailable on this platform")
}
