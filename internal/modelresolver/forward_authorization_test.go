package modelresolver

import (
	"testing"

	"github.com/egose/aiproxy/internal/config"
)

func TestCooldownFingerprintIncludesAuthorizationMode(t *testing.T) {
	model := config.Model{Name: "m", UpstreamName: "m"}
	p := config.Provider{Type: config.ProviderTypeOpenCodeZen, Name: "zen"}
	keyless := CooldownFingerprintFor("a", p, model)
	p.ForwardAuthorization = true
	forward := CooldownFingerprintFor("a", p, model)
	if keyless == forward {
		t.Fatal("changing authorization mode must invalidate existing cooldowns")
	}
}
