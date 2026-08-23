//go:build windows

package app

import "os"

func reloadSignals() []os.Signal {
	return nil
}
