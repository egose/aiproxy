package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/dbmerge"
	"github.com/egose/aiproxy/internal/store"
	"github.com/egose/aiproxy/internal/tlsprofile"
)

func tlsAdminRequest(t *testing.T, h *Handler, access, method, path string, body any) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(method, path, bytes.NewReader(raw))
	r.Header.Set("Authorization", "Bearer "+access)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	view := make(map[string]any)
	_ = json.Unmarshal(w.Body.Bytes(), &view)
	return w, view
}

func tlsAdminCleanupProvider(t *testing.T, st *store.Store, name string) {
	t.Helper()
	t.Cleanup(func() {
		if p, err := st.GetProvider(context.Background(), name); err == nil {
			_ = st.DeleteProvider(context.Background(), p.ID)
		}
	})
}

func TestApplyProviderUpsertTLSClientHelloPresence(t *testing.T) {
	h := &Handler{}
	for _, tc := range []struct {
		body string
		want string
		bad  bool
	}{
		{`{}`, "chrome", false},
		{`{"tls_client_hello":"native"}`, "native", false},
		{`{"tls_client_hello":"safari"}`, "safari", false},
		{`{"tls_client_hello":""}`, "", true},
		{`{"tls_client_hello":null}`, "", true},
		{`{"tls_client_hello":42}`, "", true},
		{`{"tls_client_hello":{"cipher_suites":[]}}`, "", true},
		{`{"tls_client_hello":"custom"}`, "", true},
		{`{"tls_client_hello":"Chrome"}`, "", true},
		{`{"tls_client_hello":"native","tls_client_hello_spec_file":null}`, "", true},
		{`{"tls_client_hello_spec_file":""}`, "", true},
		{`{"tls_client_hello_spec_file":"private-file-path"}`, "", true},
		{`{"tls_client_hello_spec":null}`, "", true},
		{`{"tls_client_hello_spec":{"private-inline-data":true}}`, "", true},
		{`{"tls_client_hello_spec_json":"private-inline-data"}`, "", true},
		{`{"TLS_CLIENT_HELLO_SPEC_FILE":null}`, "", true},
	} {
		t.Run(tc.body, func(t *testing.T) {
			row := store.DBProvider{Name: "profiled", TLSClientHello: "chrome", DisplayName: "unchanged"}
			before := row
			var req adminProviderUpsert
			if err := json.Unmarshal([]byte(tc.body), &req); err != nil {
				t.Fatal(err)
			}
			err := h.applyProviderUpsert(row.Name, &row, req)
			if tc.bad {
				if err == nil || strings.Contains(err.Error(), "private-") || !reflect.DeepEqual(row, before) {
					t.Fatalf("invalid TLS declaration mutated candidate or leaked data: %v", err)
				}
			} else if err != nil || row.TLSClientHello != tc.want {
				t.Fatalf("presence/reset mismatch: got=%s err=%v", row.TLSClientHello, err)
			}
		})
	}
}

func TestApplyProviderUpsertTLSDerivedTransitions(t *testing.T) {
	h := &Handler{}
	for _, existing := range []bool{false, true} {
		for _, declaration := range []string{`"native"`, `""`, `null`, `"custom"`} {
			row := store.DBProvider{Name: "profiled", TLSClientHello: "chrome"}
			if existing {
				row.Extends, row.TLSClientHello = "base", ""
			}
			before := row
			var req adminProviderUpsert
			if err := json.Unmarshal([]byte(`{"extends":"base","tls_client_hello":`+declaration+`}`), &req); err != nil {
				t.Fatal(err)
			}
			if err := h.applyProviderUpsert(row.Name, &row, req); err == nil || !strings.Contains(err.Error(), "derived provider cannot declare tls_client_hello") || !reflect.DeepEqual(before, row) {
				t.Fatalf("local derived declaration was accepted or mutated candidate: %v", err)
			}
		}
	}
	extends := "base"
	row := store.DBProvider{Name: "profiled", TLSClientHello: "chrome"}
	if err := h.applyProviderUpsert(row.Name, &row, adminProviderUpsert{Extends: &extends}); err != nil || row.TLSClientHello != "" || row.Extends != extends {
		t.Fatal("becoming derived must clear the omitted old local selector")
	}
	row.TLSClientHello = "native"
	if err := h.applyProviderUpsert(row.Name, &row, adminProviderUpsert{}); err != nil || row.TLSClientHello != "native" {
		t.Fatal("partial edits must not silently fix a corrupt stored derived declaration")
	}
	concrete := ""
	if err := h.applyProviderUpsert(row.Name, &row, adminProviderUpsert{Extends: &concrete, TLSClientHello: json.RawMessage(`"safari"`)}); err != nil || row.TLSClientHello != "safari" || row.Extends != "" {
		t.Fatal("becoming concrete must allow an explicit built-in selector")
	}
}

func TestProviderTLSViewMetadataDoesNotExposeCustomSource(t *testing.T) {
	data, err := os.ReadFile("../tlsprofile/testdata/bun-1.4.4-fetch.json")
	if err != nil {
		t.Fatal(err)
	}
	profile, err := tlsprofile.ParseCustom(data)
	if err != nil {
		t.Fatal(err)
	}
	p := config.Provider{Name: "static", TLSClientHello: "custom", HasTLSClientHello: true, TLSClientHelloSpecFile: "private-static-file-path", HasTLSClientHelloSpecFile: true, TLSProfile: profile, TLSClientHelloInherited: true}
	view := adminProviderView{Name: p.Name, Source: "config"}
	setProviderTLSView(&view, p)
	raw, err := json.Marshal(view)
	if err != nil || view.TLSClientHello != "custom" || view.TLSClientHelloEffective != "custom" || !view.TLSClientHelloInherited || !view.TLSClientHelloStaticOnly {
		t.Fatalf("readonly static custom metadata mismatch: %v", err)
	}
	for _, private := range []string{"private-static-file-path", "cipher_suites", "tls_client_hello_spec", profile.Identity()} {
		if bytes.Contains(raw, []byte(private)) {
			t.Fatal("view exposed custom file, contents or private descriptor")
		}
	}
	row := store.DBProvider{Name: "managed", Extends: p.Name}
	if _, err := managedProviderTLSMetadata(row, nil, config.NewCatalog([]config.Provider{p}, nil, nil)); err == nil || !strings.Contains(err.Error(), "static-config-only") || strings.Contains(err.Error(), p.TLSClientHelloSpecFile) {
		t.Fatalf("managed effective custom metadata must fail without file access: %v", err)
	}
	rows := map[string]store.DBProvider{"base": {Name: "base", TLSClientHello: "chrome"}}
	resolved, err := managedProviderTLSMetadata(store.DBProvider{Name: "managed", Extends: "base"}, rows, config.Catalog{})
	if err != nil || resolved.TLSClientHello != "chrome" || resolved.TLSProfile.CanonicalName() != "chrome-133" || !resolved.TLSClientHelloInherited {
		t.Fatalf("managed inherited view must resolve saved aliases: %v", err)
	}
	rows["base"] = store.DBProvider{Name: "base", Extends: "base"}
	if _, err := managedProviderTLSMetadata(rows["base"], rows, config.Catalog{}); err == nil {
		t.Fatal("corrupt metadata inheritance cycle was not rejected")
	}
}

func TestAdminProviderTLSClientHelloRoundTripAndActivation(t *testing.T) {
	st := openValidationStore(t)
	h, access := validationTestHandler(t, st)
	ctx := context.Background()
	deps := h.current()
	rt := &config.Runtime{Catalog: deps.Catalog}
	activations := 0
	deps.RequestReload = func() error {
		activations++
		merged, err := dbmerge.MergeCatalog(ctx, st, rt)
		if err != nil {
			return err
		}
		current := h.current()
		current.Catalog = config.NewCatalog(merged.Providers, merged.DisabledProviders, merged.Aliases)
		h.UpdateDependencies(current)
		return nil
	}
	h.UpdateDependencies(deps)
	w, metadata := tlsAdminRequest(t, h, access, http.MethodGet, "/_internal/admin/provider-types", nil)
	if w.Code != http.StatusOK {
		t.Fatal(w.Body.String())
	}
	for _, info := range metadata["provider_types"].([]any) {
		want := make([]any, 0)
		for _, selector := range tlsprofile.BuiltinSelectors() {
			want = append(want, selector)
		}
		if !reflect.DeepEqual(info.(map[string]any)["tls_client_hello_selectors"], want) {
			t.Fatal("endpoint allowlist differs from the profile registry")
		}
	}
	for _, selector := range append([]string{""}, tlsprofile.BuiltinSelectors()...) {
		name := uniqName(t, "tls-profiled")
		tlsAdminCleanupProvider(t, st, name)
		body := validProviderBody(name)
		if selector != "" {
			body["tls_client_hello"] = selector
		}
		w, view := tlsAdminRequest(t, h, access, http.MethodPost, "/_internal/admin/providers", body)
		if w.Code != http.StatusCreated || w.Header().Get(catalogSavedHeader) != "true" {
			t.Fatalf("create profile: %d %s", w.Code, w.Body.String())
		}
		want := selector
		if want == "" {
			want = "native"
		}
		profile, _ := tlsprofile.ResolveBuiltin(want)
		if view["tls_client_hello"] != want || view["tls_client_hello_effective"] != profile.CanonicalName() || view["tls_client_hello_inherited"] != false || view["tls_client_hello_static_only"] != false || strings.Contains(w.Body.String(), "tls_client_hello_spec") {
			t.Fatal("create response did not preserve declared alias and effective metadata")
		}
		active, ok := h.current().Catalog.Provider(name)
		if !ok || active.TLSProfile.Identity() != profile.Identity() {
			t.Fatal("receiving-instance reload did not activate the builtin descriptor")
		}
		row, err := st.GetProvider(ctx, name)
		if err != nil || row.TLSClientHello != selector {
			t.Fatal("database did not preserve omission or declared alias")
		}
		w, _ = tlsAdminRequest(t, h, access, http.MethodPut, "/_internal/admin/providers/"+name, map[string]any{"display_name": "Preserved"})
		if w.Code != http.StatusOK {
			t.Fatal(w.Body.String())
		}
		row, _ = st.GetProvider(ctx, name)
		if row.TLSClientHello != selector {
			t.Fatal("partial update lost the stored selector")
		}
		w, _ = tlsAdminRequest(t, h, access, http.MethodPut, "/_internal/admin/providers/"+name, map[string]any{"tls_client_hello": "native"})
		if w.Code != http.StatusOK {
			t.Fatal(w.Body.String())
		}
		w, view = tlsAdminRequest(t, h, access, http.MethodGet, "/_internal/admin/providers/"+name, nil)
		active, _ = h.current().Catalog.Provider(name)
		if w.Code != http.StatusOK || view["tls_client_hello"] != "native" || !active.TLSProfile.IsNative() {
			t.Fatal("GET and active profile did not reflect explicit native reset")
		}
	}
	if activations != 3*(len(tlsprofile.BuiltinSelectors())+1) {
		t.Fatal("valid provider mutations did not use normal local activation")
	}
}

func TestAdminProviderTLSRejectsBeforePersistence(t *testing.T) {
	st := openValidationStore(t)
	h, access := validationTestHandler(t, st)
	ctx := context.Background()
	name := uniqName(t, "tls-preserved")
	tlsAdminCleanupProvider(t, st, name)
	body := validProviderBody(name)
	body["tls_client_hello"] = "chrome"
	if w, _ := tlsAdminRequest(t, h, access, http.MethodPost, "/_internal/admin/providers", body); w.Code != http.StatusCreated {
		t.Fatal(w.Body.String())
	}
	before, err := st.GetProvider(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	beforeModels, _ := st.ListProviderModels(ctx, before.ID)
	activations := 0
	deps := h.current()
	deps.RequestReload = func() error { activations++; return nil }
	h.UpdateDependencies(deps)
	inputs := []map[string]any{
		{"tls_client_hello": nil}, {"tls_client_hello": ""}, {"tls_client_hello": "Chrome"},
		{"tls_client_hello": "firefox"}, {"tls_client_hello": "bun"}, {"tls_client_hello": "boringssl"},
		{"tls_client_hello": "chrome-155"}, {"tls_client_hello": "custom"},
		{"tls_client_hello": map[string]any{"private-inline-data": true}},
		{"tls_client_hello_spec_file": nil}, {"tls_client_hello_spec_file": ""},
		{"tls_client_hello": "custom", "tls_client_hello_spec_file": "private-file-path"},
		{"tls_client_hello": "native", "tls_client_hello_spec_file": "private-file-path"},
		{"tls_client_hello_spec": nil}, {"tls_client_hello_spec": map[string]any{"private-inline-data": true}},
		{"tls_client_hello_spec_json": "private-inline-data"},
	}
	for _, input := range inputs {
		badName := uniqName(t, "tls-rejected")
		tlsAdminCleanupProvider(t, st, badName)
		create := validProviderBody(badName)
		for key, value := range input {
			create[key] = value
		}
		for _, op := range []struct {
			method, path string
			body         any
		}{{http.MethodPost, "/_internal/admin/providers", create}, {http.MethodPut, "/_internal/admin/providers/" + name, input}} {
			w, _ := tlsAdminRequest(t, h, access, op.method, op.path, op.body)
			if w.Code != http.StatusBadRequest || w.Header().Get(catalogSavedHeader) != "" || strings.Contains(w.Body.String(), "private-") {
				t.Fatalf("rejected request persisted or leaked custom data: %d %s", w.Code, w.Body.String())
			}
		}
		if _, err := st.GetProvider(ctx, badName); !errors.Is(err, sql.ErrNoRows) {
			t.Fatal("invalid TLS create committed a row")
		}
		after, _ := st.GetProvider(ctx, name)
		afterModels, _ := st.ListProviderModels(ctx, before.ID)
		if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(beforeModels, afterModels) {
			t.Fatal("invalid TLS update committed data")
		}
	}
	if activations != 0 {
		t.Fatal("invalid TLS inputs attempted runtime activation")
	}
}

func TestAdminProviderTLSDerivedResetAndStaticCustomBoundary(t *testing.T) {
	st := openValidationStore(t)
	h, access := validationTestHandler(t, st)
	ctx := context.Background()
	deps := h.current()
	base, _ := deps.Catalog.Provider("openai")
	base.TLSClientHello, base.HasTLSClientHello = "safari", true
	base, err := config.ResolveDynamicProviderTLSProfile(base)
	if err != nil {
		t.Fatal(err)
	}
	custom := base
	custom.Name, custom.TLSClientHello, custom.TLSClientHelloSpecFile = "static-custom", "custom", "private-static-file-path"
	custom.HasTLSClientHelloSpecFile = true
	data, err := os.ReadFile("../tlsprofile/testdata/bun-1.4.4-fetch.json")
	if err != nil {
		t.Fatal(err)
	}
	custom.TLSProfile, err = tlsprofile.ParseCustom(data)
	if err != nil {
		t.Fatal(err)
	}
	deps.Catalog = config.NewCatalog([]config.Provider{base, custom}, nil, nil)
	h.UpdateDependencies(deps)
	name := uniqName(t, "tls-transition")
	tlsAdminCleanupProvider(t, st, name)
	body := validProviderBody(name)
	body["tls_client_hello"] = "chrome"
	if w, _ := tlsAdminRequest(t, h, access, http.MethodPost, "/_internal/admin/providers", body); w.Code != http.StatusCreated {
		t.Fatal(w.Body.String())
	}
	for _, selector := range []any{"native", "", nil, "custom"} {
		badName := uniqName(t, "tls-derived-local")
		tlsAdminCleanupProvider(t, st, badName)
		body := validProviderBody(badName)
		body["extends"], body["tls_client_hello"] = "openai", selector
		w, _ := tlsAdminRequest(t, h, access, http.MethodPost, "/_internal/admin/providers", body)
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "derived provider cannot declare tls_client_hello") {
			t.Fatal("derived create with explicit local TLS selector was accepted")
		}
		if _, err := st.GetProvider(ctx, badName); !errors.Is(err, sql.ErrNoRows) {
			t.Fatal("invalid derived TLS create committed")
		}
		w, _ = tlsAdminRequest(t, h, access, http.MethodPut, "/_internal/admin/providers/"+name, map[string]any{"extends": "openai", "tls_client_hello": selector})
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "derived provider cannot declare tls_client_hello") {
			t.Fatal("concrete-to-derived explicit local TLS declaration was accepted")
		}
		row, _ := st.GetProvider(ctx, name)
		if row.Extends != "" || row.TLSClientHello != "chrome" {
			t.Fatal("rejected transition changed stored provider")
		}
	}
	w, view := tlsAdminRequest(t, h, access, http.MethodPut, "/_internal/admin/providers/"+name, map[string]any{"extends": "openai"})
	row, _ := st.GetProvider(ctx, name)
	if w.Code != http.StatusOK || row.TLSClientHello != "" || row.Extends != "openai" || view["tls_client_hello"] != "safari" || view["tls_client_hello_effective"] != "safari-16.0" || view["tls_client_hello_inherited"] != true {
		t.Fatalf("omitted transition did not clear local selector and report inheritance: %d %s", w.Code, w.Body.String())
	}
	for _, input := range []map[string]any{{"tls_client_hello": "native"}, {"tls_client_hello": nil}, {"tls_client_hello_spec_file": nil}, {"extends": custom.Name}} {
		w, _ := tlsAdminRequest(t, h, access, http.MethodPut, "/_internal/admin/providers/"+name, input)
		if w.Code != http.StatusBadRequest || strings.Contains(w.Body.String(), custom.TLSClientHelloSpecFile) {
			t.Fatal("derived local declaration or inherited effective custom profile was accepted/leaked")
		}
	}
	badName := uniqName(t, "tls-custom-derived")
	tlsAdminCleanupProvider(t, st, badName)
	body = validProviderBody(badName)
	body["extends"] = custom.Name
	w, _ = tlsAdminRequest(t, h, access, http.MethodPost, "/_internal/admin/providers", body)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "static-config-only") {
		t.Fatal("managed derivative of static custom base was not rejected before commit")
	}
	if _, err := st.GetProvider(ctx, badName); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("incompatible custom derivative committed")
	}
	w, view = tlsAdminRequest(t, h, access, http.MethodGet, "/_internal/admin/providers/"+custom.Name, nil)
	if w.Code != http.StatusOK || view["source"] != "config" || view["tls_client_hello_static_only"] != true || strings.Contains(w.Body.String(), "private-static") || strings.Contains(w.Body.String(), "cipher_suites") {
		t.Fatal("static custom view did not expose only readonly/static-only metadata")
	}
}

func TestAdminProviderTLSWorkspaceAndSavedActivationBoundary(t *testing.T) {
	st := openValidationStore(t)
	h, global := validationTestHandler(t, st)
	ctx := context.Background()
	workspace := mkWorkspace(t, st, h, global, "tls-workspace")
	_, admin := mkWorkspaceMember(t, st, h, global, uniqName(t, "tls-owner")+"@example.test", workspace, "admin")
	_, member := mkWorkspaceMember(t, st, h, global, uniqName(t, "tls-member")+"@example.test", workspace, "member")
	name := uniqName(t, "tls-workspace-provider")
	tlsAdminCleanupProvider(t, st, name)
	body := validProviderBody(name)
	body["workspace_id"], body["tls_client_hello"] = workspace, "chrome"
	w, _ := tlsAdminRequest(t, h, admin, http.MethodPost, "/_internal/admin/providers", body)
	if w.Code != http.StatusCreated {
		t.Fatal(w.Body.String())
	}
	for _, access := range []string{global, admin} {
		w, _ = tlsAdminRequest(t, h, access, http.MethodPut, "/_internal/admin/providers/"+name, map[string]any{"tls_client_hello": "custom", "tls_client_hello_spec_file": "private-file-path"})
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "static-config-only") || strings.Contains(w.Body.String(), "private-file-path") {
			t.Fatal("an admin role bypassed the static-only custom source boundary")
		}
	}
	w, _ = tlsAdminRequest(t, h, member, http.MethodPut, "/_internal/admin/providers/"+name, map[string]any{"tls_client_hello": "native"})
	if w.Code != http.StatusBadRequest && w.Code != http.StatusForbidden {
		t.Fatal("TLS edit bypassed workspace write authorization")
	}
	row, _ := st.GetProvider(ctx, name)
	if row.TLSClientHello != "chrome" {
		t.Fatal("rejected custom/unauthorized edits changed stored selector")
	}
	for _, input := range []map[string]any{
		{"api_key": "rejected-key", "tls_client_hello_spec_file": nil},
		{"api_key": "rejected-key", "tls_client_hello_spec_file": "private-file-path"},
		{"api_key": "rejected-key", "tls_client_hello_spec_json": "private-inline-data"},
		{"api_key": "rejected-key", "tls_client_hello": "native"},
	} {
		w, _ = tlsAdminRequest(t, h, admin, http.MethodPut, "/_internal/admin/providers/"+name+"/credential", input)
		if w.Code != http.StatusBadRequest || w.Header().Get(catalogSavedHeader) != "" || strings.Contains(w.Body.String(), "private-") {
			t.Fatal("credential route accepted unsupported TLS declarations")
		}
		after, _ := st.GetProvider(ctx, name)
		if !reflect.DeepEqual(row, after) {
			t.Fatal("unsupported credential-route TLS declaration committed credentials")
		}
	}
	deps := h.current()
	deps.RequestReload = func() error { return errors.New("synthetic activation failure") }
	h.UpdateDependencies(deps)
	w, _ = tlsAdminRequest(t, h, admin, http.MethodPut, "/_internal/admin/providers/"+name, map[string]any{"tls_client_hello": "native"})
	row, _ = st.GetProvider(ctx, name)
	if w.Code != http.StatusInternalServerError || w.Header().Get(catalogSavedHeader) != "true" || row.TLSClientHello != "native" || !strings.Contains(w.Body.String(), "saved") {
		t.Fatal("TLS mutation changed the saved-versus-activated contract")
	}
}

func tlsJSONNameFoldDeclarations() []struct {
	name, field string
	value       any
} {
	return []struct {
		name, field string
		value       any
	}{
		{"canonical", "tls_client_hello_spec_file", "unused.json"},
		{"canonical-null", "tls_client_hello_spec_file", nil},
		{"canonical-empty", "tls_client_hello_spec_file", ""},
		{"mixed", "TlS_ClIeNt_HeLlO_SpEc_FiLe", "unused.json"},
		{"long-s", "tl\u017f_client_hello_spec_file", "unused.json"},
		{"long-s-null", "tl\u017f_client_hello_spec_file", nil},
		{"long-s-empty", "tl\u017f_client_hello_spec_file", ""},
		{"mixed-long-s", "TL\u017f_ClIeNt_HeLlO_\u017fPeC_FiLe", nil},
		{"canonical-inline", "tls_client_hello_spec_json", nil},
		{"mixed-inline", "TlS_ClIeNt_HeLlO_SpEc_JsOn", "inline-data"},
		{"long-s-inline-null", "tl\u017f_client_hello_spec_json", nil},
		{"long-s-inline", "tl\u017f_client_hello_spec_json", map[string]any{"cipher_suites": []string{}}},
		{"canonical-custom", "tls_client_hello", "custom"},
		{"mixed-custom", "TlS_ClIeNt_HeLlO", "custom"},
		{"long-s-custom", "tl\u017f_client_hello", "custom"},
		{"long-s-selector-null", "tl\u017f_client_hello", nil},
		{"long-s-selector-empty", "tl\u017f_client_hello", ""},
	}
}

func TestJSONFieldHasPrefixSimpleFold(t *testing.T) {
	for _, tc := range []struct {
		name, prefix string
		want         bool
	}{
		{"tls_client_hello_spec_file", "tls_client_hello", true},
		{"TlS_ClIeNt_HeLlO_SpEc_FiLe", "tls_client_hello", true},
		{"tl\u017f_client_hello_spec_file", "tls_client_hello", true},
		{"tl\u017f_client_hello", "tls_client_hello", true},
		{"tlss_client_hello_spec_file", "tls_client_hello", false},
		{"tls_client_hell\u03bf_spec_file", "tls_client_hello", false},
		{"x_tls_client_hello_spec_file", "tls_client_hello", false},
		{"tls_client_hell", "tls_client_hello", false},
		{"", "tls_client_hello", false},
		{"\u017f_suffix", "s", true},
		{"S_suffix", "\u017f", true},
		{"\u212a_suffix", "k", true},
		{"\u00df_suffix", "ss", false},
	} {
		t.Run(tc.name+"/"+tc.prefix, func(t *testing.T) {
			if got := jsonFieldHasPrefix(tc.name, tc.prefix); got != tc.want {
				t.Fatalf("simple-fold prefix match = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestAdminCredentialTLSJSONNameFoldRejectsBeforeCommit(t *testing.T) {
	st := openValidationStore(t)
	h, global := validationTestHandler(t, st)
	ctx := context.Background()
	workspace := mkWorkspace(t, st, h, global, "tls-fold")
	_, admin := mkWorkspaceMember(t, st, h, global, uniqName(t, "tls-fold-owner")+"@example.test", workspace, "admin")
	for _, role := range []struct{ name, access string }{{"global", global}, {"workspace-admin", admin}} {
		for _, declaration := range tlsJSONNameFoldDeclarations() {
			t.Run(role.name+"/"+declaration.name, func(t *testing.T) {
				name := uniqName(t, "tls-fold-credential")
				tlsAdminCleanupProvider(t, st, name)
				body := validProviderBody(name)
				body["workspace_id"], body["tls_client_hello"] = workspace, "chrome"
				w, _ := tlsAdminRequest(t, h, role.access, http.MethodPost, "/_internal/admin/providers", body)
				if w.Code != http.StatusCreated {
					t.Fatal(w.Body.String())
				}
				before, err := st.GetProvider(ctx, name)
				if err != nil {
					t.Fatal(err)
				}
				var update any = map[string]any{"api_key": "replacement", declaration.field: declaration.value}
				if declaration.name == "long-s" {
					update = json.RawMessage(`{"api_key":"replacement","tl\u017f_client_hello_spec_file":"unused.json"}`)
				}
				w, _ = tlsAdminRequest(t, h, role.access, http.MethodPut, "/_internal/admin/providers/"+name+"/credential", update)
				if w.Code != http.StatusBadRequest || w.Header().Get(catalogSavedHeader) != "" {
					t.Errorf("unsupported TLS declaration was silently accepted: status=%d saved=%q", w.Code, w.Header().Get(catalogSavedHeader))
				}
				after, err := st.GetProvider(ctx, name)
				if err != nil {
					t.Fatal(err)
				}
				if !before.UpdatedAt.Equal(after.UpdatedAt) || !bytes.Equal(before.APIKeyEncrypted, after.APIKeyEncrypted) || !reflect.DeepEqual(before, after) {
					t.Error("rejected TLS declaration changed revision or encrypted credentials")
				}
			})
		}
	}
}

func TestAdminProviderTLSJSONNameFoldForbiddenDeclarations(t *testing.T) {
	st := openValidationStore(t)
	h, access := validationTestHandler(t, st)
	ctx := context.Background()
	for _, declaration := range tlsJSONNameFoldDeclarations() {
		t.Run(declaration.name, func(t *testing.T) {
			name := uniqName(t, "tls-fold-provider")
			tlsAdminCleanupProvider(t, st, name)
			if w, _ := tlsAdminRequest(t, h, access, http.MethodPost, "/_internal/admin/providers", validProviderBody(name)); w.Code != http.StatusCreated {
				t.Fatal(w.Body.String())
			}
			before, err := st.GetProvider(ctx, name)
			if err != nil {
				t.Fatal(err)
			}
			w, _ := tlsAdminRequest(t, h, access, http.MethodPut, "/_internal/admin/providers/"+name, map[string]any{"api_key": "replacement", declaration.field: declaration.value})
			if w.Code != http.StatusBadRequest || w.Header().Get(catalogSavedHeader) != "" {
				t.Errorf("provider route silently accepted forbidden TLS alias: status=%d saved=%q", w.Code, w.Header().Get(catalogSavedHeader))
			}
			after, err := st.GetProvider(ctx, name)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Errorf("forbidden provider TLS alias committed an update: %v", err)
			}
			badName := uniqName(t, "tls-fold-create")
			tlsAdminCleanupProvider(t, st, badName)
			body := validProviderBody(badName)
			body[declaration.field] = declaration.value
			w, _ = tlsAdminRequest(t, h, access, http.MethodPost, "/_internal/admin/providers", body)
			if w.Code != http.StatusBadRequest || w.Header().Get(catalogSavedHeader) != "" {
				t.Errorf("provider create silently accepted forbidden TLS alias: status=%d saved=%q", w.Code, w.Header().Get(catalogSavedHeader))
			}
			if _, err := st.GetProvider(ctx, badName); !errors.Is(err, sql.ErrNoRows) {
				t.Error("forbidden provider TLS alias committed a new provider")
			}
		})
	}
}

func TestAdminTLSJSONNameFoldPreservesAllowedAndUnknownFields(t *testing.T) {
	st := openValidationStore(t)
	h, access := validationTestHandler(t, st)
	ctx := context.Background()
	for _, field := range []string{"tls_client_hello", "TlS_ClIeNt_HeLlO", "tl\u017f_client_hello"} {
		t.Run(field, func(t *testing.T) {
			name := uniqName(t, "tls-fold-selector")
			tlsAdminCleanupProvider(t, st, name)
			body := validProviderBody(name)
			body[field] = "chrome"
			w, _ := tlsAdminRequest(t, h, access, http.MethodPost, "/_internal/admin/providers", body)
			if w.Code != http.StatusCreated {
				t.Fatal(w.Body.String())
			}
			w, _ = tlsAdminRequest(t, h, access, http.MethodPut, "/_internal/admin/providers/"+name, map[string]any{field: "native"})
			row, err := st.GetProvider(ctx, name)
			if w.Code != http.StatusOK || err != nil || row.TLSClientHello != "native" {
				t.Fatal("valid selector aliases no longer follow encoding/json field matching")
			}
		})
	}
	for _, field := range []string{"x_tls_client_hello_spec_file", "tlss_client_hello_spec_file", "tls_client_hell\u03bf_spec_file", "unrelated_setting"} {
		t.Run(field, func(t *testing.T) {
			name := uniqName(t, "tls-fold-unknown")
			tlsAdminCleanupProvider(t, st, name)
			if w, _ := tlsAdminRequest(t, h, access, http.MethodPost, "/_internal/admin/providers", validProviderBody(name)); w.Code != http.StatusCreated {
				t.Fatal(w.Body.String())
			}
			w, _ := tlsAdminRequest(t, h, access, http.MethodPut, "/_internal/admin/providers/"+name+"/credential", map[string]any{"api_key": "replacement", field: "unused.json"})
			if w.Code != http.StatusOK || w.Header().Get(catalogSavedHeader) != "true" {
				t.Fatal("unrelated unknown-field semantics changed")
			}
			row, err := st.GetProvider(ctx, name)
			if err != nil {
				t.Fatal(err)
			}
			plain, err := store.DecryptSecret(row.APIKeyEncrypted)
			if err != nil || string(plain) != "replacement" {
				t.Fatal("ordinary credential updates changed")
			}
		})
	}
}
