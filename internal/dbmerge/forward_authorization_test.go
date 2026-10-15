package dbmerge

import (
	"testing"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/store"
)

func TestBuildProviderForwardAuthorization(t *testing.T) {
	for _, extends := range []string{"", "base"} {
		t.Run("extends="+extends, func(t *testing.T) {
			row := store.DBProvider{Name: "forward", Type: "openai", Enabled: true, Extends: extends, ForwardAuthorization: true}
			models := []store.DBProviderModel{{Name: "m", Capabilities: []string{"chat"}}}
			p, err := BuildProvider(row, models, testBases())
			if err != nil {
				t.Fatal(err)
			}
			if !p.ForwardAuthorization || p.APIKey != "" || p.APIKeyRef != nil {
				t.Fatal("unexpected authorization mode")
			}
			if err := config.ValidateDynamicProvider(p); err != nil {
				t.Fatal(err)
			}
			row.APIKeyRefKey = "key"
			if _, err := BuildProvider(row, models, testBases()); err == nil {
				t.Fatal("forwarding with stored credentials must fail")
			}
		})
	}
}

func TestBuildProviderDoesNotInheritForwardAuthorization(t *testing.T) {
	bases := testBases()
	base := bases["base"]
	base.APIKey = ""
	base.ForwardAuthorization = true
	bases["base"] = base
	p, err := BuildProvider(store.DBProvider{Name: "derived", Type: "openai", Extends: "base", Enabled: true}, nil, bases)
	if err != nil {
		t.Fatal(err)
	}
	if p.ForwardAuthorization {
		t.Fatal("forwarding must not be inherited implicitly")
	}
	if err := config.ValidateDynamicProvider(p); err == nil {
		t.Fatal("derived provider without a local credential must fail")
	}
}
