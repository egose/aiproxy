package main

import "fmt"

func deferredConfigureUnsupported() error {
	return fmt.Errorf("peer deferred settings have no CLI creation flags in v1; edit deferred, deferred_transport, and deferred_receiver blocks in the config file directly")
}
