package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestManagedDeferredModelPresenceRejectedAcrossBothViews(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, empty := range []bool{false, true} {
			for _, view := range []string{"ordered", "indexed", "index-only", "ordered-with-nil-index", "indexed-with-nil-ordered", "conflicting", "indexed-extra"} {
				t.Run(strings.Join([]string{view, map[bool]string{false: "disabled", true: "enabled"}[enabled], map[bool]string{false: "configured", true: "empty"}[empty]}, "/"), func(t *testing.T) {
					options := DefaultDeferredTransport()
					if empty {
						options = DeferredTransport{}
					}
					ordinary := Model{Name: "m", UpstreamName: "m"}
					opted := ordinary
					opted.DeferredTransport = &options
					p := Provider{
						Name: "managed", Type: ProviderTypeOpenAICompatible, Enabled: enabled,
						BaseURL: "https://peer.example/v1", APIKey: "configured-key",
						Models: []Model{ordinary}, ModelByName: map[string]Model{"m": ordinary},
					}
					switch view {
					case "ordered":
						p.Models, p.ModelByName = []Model{opted}, nil
					case "indexed":
						p.Models = nil
						p.ModelByName["m"] = opted
					case "index-only":
						p.Models = nil
						p.ModelByName = map[string]Model{"not-in-models": {DeferredTransport: &options}}
					case "ordered-with-nil-index":
						p.Models[0] = opted
					case "indexed-with-nil-ordered":
						p.ModelByName["m"] = opted
					case "conflicting":
						p.Models[0] = opted
						other := DeferredTransport{}
						ordinary.DeferredTransport = &other
						p.ModelByName["m"] = ordinary
					case "indexed-extra":
						p.ModelByName["extra"] = opted
					}
					before := NewCatalog([]Provider{p}, nil, nil).Providers()
					for _, validate := range []func(Provider) error{ValidateManagedProviderDeferred, ValidateDynamicProvider} {
						if err := validate(p); err == nil || !strings.Contains(err.Error(), "static-config-only") {
							t.Fatalf("managed caller presence was hidden before validation: %v", err)
						}
					}
					if after := NewCatalog([]Provider{p}, nil, nil).Providers(); !reflect.DeepEqual(before, after) {
						t.Fatal("managed rejection mutated caller policy")
					}
				})
			}
		}
	}
	p := validDynamicProvider()
	if err := ValidateManagedProviderDeferred(p); err != nil {
		t.Fatalf("ordinary managed policy rejected: %v", err)
	}
	if err := ValidateDynamicProvider(p); err != nil {
		t.Fatalf("ordinary managed provider changed: %v", err)
	}
}
