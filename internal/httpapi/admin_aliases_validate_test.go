package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func validAliasBody(name string) map[string]interface{} {
	return map[string]interface{}{
		"name":      name,
		"algorithm": "round_robin",
		"targets": []interface{}{
			map[string]interface{}{"provider": "openai", "model": "gpt-4o-mini"},
		},
	}
}

func TestAdminCreateAliasValidation(t *testing.T) {
	st := openValidationStore(t)
	h, access := validationTestHandler(t, st)

	cases := []struct {
		name   string
		mutate func(map[string]interface{})
		want   string
	}{
		{"bad name", func(b map[string]interface{}) { b["name"] = "Bad Name" }, "lowercase"},
		{"static collision", func(b map[string]interface{}) { b["name"] = "chat_default" }, "static config"},
		{"bad algorithm", func(b map[string]interface{}) { b["algorithm"] = "random" }, "round_robin or least_connections"},
		{"no targets", func(b map[string]interface{}) { b["targets"] = []interface{}{} }, "at least one target"},
		{"shorthand mixing", func(b map[string]interface{}) {
			b["providers"] = []string{"openai"}
			b["model"] = "gpt-4o-mini"
		}, "cannot be combined"},
		{"shorthand needs model", func(b map[string]interface{}) {
			delete(b, "targets")
			b["providers"] = []string{"openai"}
		}, "must be set together"},
		{"unknown provider", func(b map[string]interface{}) {
			b["targets"] = []interface{}{map[string]interface{}{"provider": "ghost", "model": "m"}}
		}, `target provider "ghost" is not defined`},
		{"unknown model", func(b map[string]interface{}) {
			b["targets"] = []interface{}{map[string]interface{}{"provider": "openai", "model": "ghost"}}
		}, "is not defined on provider"},
		{"dup targets", func(b map[string]interface{}) {
			b["targets"] = []interface{}{
				map[string]interface{}{"provider": "openai", "model": "gpt-4o-mini"},
				map[string]interface{}{"provider": "openai", "model": "gpt-4o-mini"},
			}
		}, "duplicate target"},
		{"bad retry code", func(b map[string]interface{}) { b["retry_status_codes"] = []int{200} }, "between 400 and 599"},
		{"bad affinity header", func(b map[string]interface{}) {
			b["session_affinity"] = map[string]interface{}{"headers": []string{"not a header!"}}
		}, "invalid header name"},
		{"bad reasoning action", func(b map[string]interface{}) {
			b["encrypted_reasoning"] = map[string]interface{}{"on_caller_mismatch": "nope"}
		}, "must be"},
	}
	for _, tc := range cases {
		body := validAliasBody(uniqName(t, "valias"))
		tc.mutate(body)
		code, _, raw := doAdmin(t, h, access, http.MethodPost, "/_internal/admin/aliases", body)
		if code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", tc.name, code)
			continue
		}
		if !strings.Contains(raw, tc.want) {
			t.Errorf("%s: body = %q, want substring %q", tc.name, raw, tc.want)
		}
	}
}

func TestAdminCreateAliasDefaults(t *testing.T) {
	st := openValidationStore(t)
	h, access := validationTestHandler(t, st)
	ctx := context.Background()
	name := uniqName(t, "dalias")

	code, body, _ := doAdmin(t, h, access, http.MethodPost, "/_internal/admin/aliases", validAliasBody(name))
	if code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", code)
	}
	t.Cleanup(func() {
		if a, err := st.GetAlias(ctx, name); err == nil {
			_ = st.DeleteAlias(ctx, a.ID)
		}
	})
	codes, ok := body["retry_status_codes"].([]interface{})
	if !ok || len(codes) != 4 {
		t.Fatalf("retry_status_codes = %v, want 4 defaults", body["retry_status_codes"])
	}
}

func TestAdminCreateAliasShorthand(t *testing.T) {
	st := openValidationStore(t)
	h, access := validationTestHandler(t, st)
	ctx := context.Background()
	name := uniqName(t, "salias")

	code, body, _ := doAdmin(t, h, access, http.MethodPost, "/_internal/admin/aliases", map[string]interface{}{
		"name": name, "algorithm": "least_connections",
		"providers": []string{"openai"}, "model": "gpt-4o-mini",
	})
	if code != http.StatusCreated {
		t.Fatalf("shorthand status = %d, want 201", code)
	}
	t.Cleanup(func() {
		if a, err := st.GetAlias(ctx, name); err == nil {
			_ = st.DeleteAlias(ctx, a.ID)
		}
	})
	targets, ok := body["targets"].([]interface{})
	if !ok || len(targets) != 1 {
		t.Fatalf("targets = %v, want 1 expanded target", body["targets"])
	}
}

func TestAdminUpdateAliasValidation(t *testing.T) {
	st := openValidationStore(t)
	h, access := validationTestHandler(t, st)
	ctx := context.Background()
	name := uniqName(t, "ualias")

	code, _, _ := doAdmin(t, h, access, http.MethodPost, "/_internal/admin/aliases", validAliasBody(name))
	if code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", code)
	}
	t.Cleanup(func() {
		if a, err := st.GetAlias(ctx, name); err == nil {
			_ = st.DeleteAlias(ctx, a.ID)
		}
	})

	code, _, raw := doAdmin(t, h, access, http.MethodPut, "/_internal/admin/aliases/"+name, map[string]interface{}{
		"algorithm": "random",
	})
	if code != http.StatusBadRequest || !strings.Contains(raw, "round_robin or least_connections") {
		t.Errorf("bad algorithm update: status = %d, body = %q", code, raw)
	}

	code, body, _ := doAdmin(t, h, access, http.MethodPut, "/_internal/admin/aliases/"+name, map[string]interface{}{
		"algorithm": "least_connections",
	})
	if code != http.StatusOK {
		t.Fatalf("update status = %d, want 200", code)
	}
	if body["algorithm"] != "least_connections" {
		t.Errorf("algorithm = %v, want least_connections", body["algorithm"])
	}
	if targets, ok := body["targets"].([]interface{}); !ok || len(targets) != 1 {
		t.Errorf("targets = %v, want 1 preserved target", body["targets"])
	}
}

func TestAdminAliasDottedName(t *testing.T) {
	st := openValidationStore(t)
	h, access := validationTestHandler(t, st)
	ctx := context.Background()
	name := uniqName(t, "my.alias")

	code, _, _ := doAdmin(t, h, access, http.MethodPost, "/_internal/admin/aliases", validAliasBody(name))
	if code != http.StatusCreated {
		t.Fatalf("dotted name status = %d, want 201", code)
	}
	t.Cleanup(func() {
		if a, err := st.GetAlias(ctx, name); err == nil {
			_ = st.DeleteAlias(ctx, a.ID)
		}
	})
}

func TestAdminAliasDisabledTarget(t *testing.T) {
	st := openValidationStore(t)
	h, access := validationTestHandler(t, st)
	ctx := context.Background()
	prov := uniqName(t, "disprov")

	code, _, _ := doAdmin(t, h, access, http.MethodPost, "/_internal/admin/providers", map[string]interface{}{
		"name": prov, "type": "openai", "api_key": "secret", "enabled": false,
		"models": []interface{}{map[string]interface{}{"name": "m"}},
	})
	if code != http.StatusCreated {
		t.Fatalf("provider create status = %d, want 201", code)
	}
	t.Cleanup(func() {
		if p, err := st.GetProvider(ctx, prov); err == nil {
			_ = st.DeleteProvider(ctx, p.ID)
		}
	})

	code, _, raw := doAdmin(t, h, access, http.MethodPost, "/_internal/admin/aliases", map[string]interface{}{
		"name": uniqName(t, "disalias"), "algorithm": "round_robin",
		"targets": []interface{}{map[string]interface{}{"provider": prov, "model": "m"}},
	})
	if code != http.StatusBadRequest || !strings.Contains(raw, "disabled") {
		t.Errorf("disabled target: status = %d, body = %q, want disabled error", code, raw)
	}
}
