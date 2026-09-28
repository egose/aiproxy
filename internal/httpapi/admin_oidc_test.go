package httpapi

import (
	"testing"

	"github.com/egose/aiproxy/internal/store"
)

func TestOIDCAdminMatch(t *testing.T) {
	cfg := store.OIDCConfig{AdminClaim: "groups", AdminValue: "aiproxy-admins"}
	cases := []struct {
		name   string
		claims map[string]interface{}
		want   bool
	}{
		{"string match", map[string]interface{}{"groups": "aiproxy-admins"}, true},
		{"string miss", map[string]interface{}{"groups": "other"}, false},
		{"list membership", map[string]interface{}{"groups": []interface{}{"users", "aiproxy-admins"}}, true},
		{"list miss", map[string]interface{}{"groups": []interface{}{"users"}}, false},
		{"missing claim", map[string]interface{}{}, false},
		{"bool true", map[string]interface{}{"admin": true}, false},
	}
	for _, tc := range cases {
		if got := oidcAdminMatch(tc.claims, cfg); got != tc.want {
			t.Errorf("%s = %v, want %v", tc.name, got, tc.want)
		}
	}
	boolCfg := store.OIDCConfig{AdminClaim: "admin", AdminValue: "true"}
	if !oidcAdminMatch(map[string]interface{}{"admin": true}, boolCfg) {
		t.Errorf("bool match = false, want true")
	}
	strCfg := store.OIDCConfig{AdminClaim: "role", AdminValue: "admin"}
	if !oidcAdminMatch(map[string]interface{}{"role": "admin"}, strCfg) {
		t.Errorf("string match = false, want true")
	}
	emptyCfg := store.OIDCConfig{}
	if oidcAdminMatch(map[string]interface{}{"groups": []interface{}{"x"}}, emptyCfg) {
		t.Errorf("empty claim config matched, want false")
	}
}

func TestOIDCClaimString(t *testing.T) {
	claims := map[string]interface{}{"email": "a@b.c", "n": 1}
	if got := oidcClaimString(claims, "email"); got != "a@b.c" {
		t.Fatalf("email = %q", got)
	}
	if got := oidcClaimString(claims, "missing"); got != "" {
		t.Fatalf("missing = %q, want empty", got)
	}
	if got := oidcClaimString(claims, "n"); got != "" {
		t.Fatalf("non-string = %q, want empty", got)
	}
}

func TestOIDCEnabled(t *testing.T) {
	if oidcEnabled(store.OIDCConfig{Enabled: true}) {
		t.Fatalf("incomplete config reports enabled")
	}
	if !oidcEnabled(store.OIDCConfig{Enabled: true, IssuerURL: "http://x/realms/a", ClientID: "c"}) {
		t.Fatalf("complete config reports disabled")
	}
	if oidcEnabled(store.OIDCConfig{Enabled: false, IssuerURL: "http://x", ClientID: "c"}) {
		t.Fatalf("disabled flag reports enabled")
	}
}
