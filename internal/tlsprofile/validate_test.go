package tlsprofile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	utls "github.com/refraction-networking/utls"
)

func TestStrictRecipeJSON(t *testing.T) {
	fixture := fixtureData(t)
	for _, tc := range []struct {
		name string
		data []byte
		want string
	}{
		{"empty", nil, "invalid JSON"},
		{"missing arrays", []byte(`{}`), "cipher_suites"},
		{"root array", []byte(`[]`), "one JSON object"},
		{"root null", []byte(`null`), "one JSON object"},
		{"root string", []byte(`"private-content-value"`), "one JSON object"},
		{"trailing object", append(slices.Clone(fixture), []byte(`{}`)...), "trailing data"},
		{"trailing scalar", append(slices.Clone(fixture), []byte(`false`)...), "trailing data"},
		{"trailing comment", append(slices.Clone(fixture), []byte(`/* private-content-value */`)...), "trailing data"},
		{"truncated", fixture[:len(fixture)-3], "invalid JSON"},
		{"invalid utf8", []byte{'{', '"', 0xff, '"', ':', '1', '}'}, "valid UTF-8"},
		{"bom", append([]byte{0xef, 0xbb, 0xbf}, fixture...), "invalid JSON"},
		{"duplicate root", []byte(`{"cipher_suites":[],"cipher_suites":[]}`), "cipher_suites: duplicate object key"},
		{"escaped duplicate", []byte(`{"cipher_suites":[],"cipher_\u0073uites":[]}`), "duplicate object key"},
		{"duplicate extension", []byte(`{"extensions":[{"name":"server_name","name":"server_name"}]}`), "extensions[0].name: duplicate object key"},
		{"duplicate share", []byte(`{"extensions":[{"client_shares":[{"group":"x25519","group":"x25519"}]}]}`), "extensions[0].client_shares[0].group: duplicate object key"},
		{"duplicate unknown", []byte(`{"private-content-value":1,"private-content-value":2}`), "duplicate object key"},
		{"env inside json", []byte(`{"cipher_suites":[env("TLS_CIPHER")]}`), "invalid JSON"},
		{"over size", bytes.Repeat([]byte{' '}, MaxSpecBytes+1), "65536 bytes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseCustom(tc.data)
			if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "private-content-value") {
				t.Fatalf("error = %v, want %q without arbitrary content", err, tc.want)
			}
			if _, err := DecodeSpec(tc.data); err == nil {
				t.Fatal("DecodeSpec bypassed strict validation")
			}
		})
	}
	for _, data := range [][]byte{fixture, append(append([]byte(" \t\r\n"), fixture...), []byte(" \t\r\n")...), append(fixture, bytes.Repeat([]byte{' '}, MaxSpecBytes-len(fixture))...)} {
		if _, err := ParseCustom(data); err != nil {
			t.Fatalf("valid whitespace/size boundary rejected: %v", err)
		}
	}
}

func TestJSONStructuralBounds(t *testing.T) {
	for _, depth := range []int{8, 9} {
		data := []byte(`{"unknown":` + strings.Repeat("[", depth-1) + `0` + strings.Repeat("]", depth-1) + `}`)
		_, err := readRecipe(data)
		if (err == nil) != (depth == 8) {
			t.Fatalf("depth %d: %v", depth, err)
		}
	}
	for _, entries := range []int{4091, 4092} {
		data := []byte(`{"unknown":[` + strings.Repeat("0,", entries-1) + `0]}`)
		_, err := readRecipe(data)
		if (err == nil) != (entries == 4091) {
			t.Fatalf("tokens for %d entries: %v", entries, err)
		}
		if err != nil && !strings.Contains(err.Error(), "4096-token") {
			t.Fatalf("wrong token-bound error: %v", err)
		}
	}
	for _, limit := range []int{1, 3, 16, 32, 64} {
		for _, count := range []int{0, 1, limit, limit + 1} {
			values := make([]any, count)
			for i := range values {
				values[i] = "entry"
			}
			_, err := recipeStrings(values, "field", 1, limit)
			if (err == nil) != (count >= 1 && count <= limit) {
				t.Fatalf("array %d bound %d: %v", count, limit, err)
			}
		}
	}
}

func TestStrictRecipeFields(t *testing.T) {
	mutateExtension := func(name, key string, value any) func(map[string]any) {
		return func(recipe map[string]any) { recipeExtension(t, recipe, name)[key] = value }
	}
	share := func(group string, fields ...any) map[string]any {
		result := map[string]any{"group": group}
		if len(fields) > 0 {
			result["key_exchange"] = fields[0]
		}
		return result
	}
	cases := []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{"unknown root", func(r map[string]any) { r["private-content-value"] = "private-content-value" }, "spec: unknown field"},
		{"missing suites", func(r map[string]any) { delete(r, "cipher_suites") }, "cipher_suites"},
		{"null suites", func(r map[string]any) { r["cipher_suites"] = nil }, "cipher_suites"},
		{"empty suites", func(r map[string]any) { r["cipher_suites"] = []any{} }, "1..64"},
		{"too many suites", func(r map[string]any) { r["cipher_suites"] = make([]any, 65) }, "1..64"},
		{"numeric suite", func(r map[string]any) { r["cipher_suites"] = []any{4865} }, "cipher_suites[0]"},
		{"empty suite", func(r map[string]any) { r["cipher_suites"] = []string{""} }, "cipher_suites[0]"},
		{"null suite", func(r map[string]any) { r["cipher_suites"] = []any{nil} }, "cipher_suites[0]"},
		{"unknown suite", func(r map[string]any) { r["cipher_suites"].([]any)[0] = "private-content-value" }, "cipher_suites[0]"},
		{"duplicate suite", func(r map[string]any) { r["cipher_suites"].([]any)[1] = r["cipher_suites"].([]any)[0] }, "cipher_suites[1]: duplicate"},
		{"duplicate grease suite", func(r map[string]any) { r["cipher_suites"] = []string{"GREASE", "GREASE"} }, "duplicate"},
		{"only legacy suites", func(r map[string]any) { r["cipher_suites"] = []string{"TLS_RSA_WITH_AES_128_CBC_SHA"} }, "safe usable suite"},
		{"only tls13 suites", func(r map[string]any) { r["cipher_suites"] = []string{"TLS_AES_128_GCM_SHA256"} }, "safe usable suite"},
		{"only tls12 suites", func(r map[string]any) { r["cipher_suites"] = []string{"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"} }, "safe usable suite"},
		{"missing compression", func(r map[string]any) { delete(r, "compression_methods") }, "compression_methods"},
		{"null compression", func(r map[string]any) { r["compression_methods"] = nil }, "compression_methods"},
		{"empty compression", func(r map[string]any) { r["compression_methods"] = []string{} }, "compression_methods"},
		{"extra compression", func(r map[string]any) { r["compression_methods"] = []string{"NULL", "NULL"} }, "compression_methods"},
		{"compressed", func(r map[string]any) { r["compression_methods"] = []string{"DEFLATE"} }, "only NULL"},
		{"numeric compression", func(r map[string]any) { r["compression_methods"] = []any{0} }, "compression_methods[0]"},
		{"missing extensions", func(r map[string]any) { delete(r, "extensions") }, "extensions"},
		{"null extensions", func(r map[string]any) { r["extensions"] = nil }, "extensions"},
		{"empty extensions", func(r map[string]any) { r["extensions"] = []any{} }, "1..32"},
		{"too many extensions", func(r map[string]any) { r["extensions"] = make([]any, 33) }, "1..32"},
		{"null extension", func(r map[string]any) { r["extensions"].([]any)[0] = nil }, "extensions[0]"},
		{"missing extension name", func(r map[string]any) { r["extensions"].([]any)[0] = map[string]any{} }, "extensions[0].name"},
		{"empty extension name", mutateExtension("server_name", "name", ""), "extensions[0].name"},
		{"duplicate extension", mutateExtension("extended_master_secret", "name", "server_name"), "duplicate extension"},
		{"missing sni", func(r map[string]any) { removeExtensions(r, "server_name") }, "requires server_name"},
		{"missing groups", func(r map[string]any) { removeExtensions(r, "supported_groups") }, "requires supported_groups"},
		{"missing signatures", func(r map[string]any) { removeExtensions(r, "signature_algorithms") }, "requires signature_algorithms"},
		{"fixed sni", mutateExtension("server_name", "server_name", "private-content-value"), "extensions[0]: unknown field"},
		{"fixed ticket", mutateExtension("session_ticket", "ticket", "private-content-value"), "extensions[5]: unknown field"},
		{"fixed renegotiation", mutateExtension("renegotiation_info", "renegotiated_connection", "private-content-value"), "unknown field"},
		{"null groups", mutateExtension("supported_groups", "named_group_list", nil), "named_group_list"},
		{"empty groups", mutateExtension("supported_groups", "named_group_list", []string{}), "named_group_list"},
		{"too many groups", mutateExtension("supported_groups", "named_group_list", make([]any, 17)), "1..16"},
		{"numeric group", mutateExtension("supported_groups", "named_group_list", []any{29}), "named_group_list[0]"},
		{"unknown group", mutateExtension("supported_groups", "named_group_list", []string{"private-content-value"}), "unsupported named group"},
		{"duplicate group", mutateExtension("supported_groups", "named_group_list", []string{"x25519", "x25519"}), "duplicate entry"},
		{"only grease group", mutateExtension("supported_groups", "named_group_list", []string{"GREASE"}), "real supported group"},
		{"duplicate grease group", mutateExtension("supported_groups", "named_group_list", []string{"GREASE", "GREASE", "x25519"}), "duplicate entry"},
		{"only mlkem group", mutateExtension("supported_groups", "named_group_list", []string{"X25519MLKEM768"}), "classical group"},
		{"null points", mutateExtension("ec_point_formats", "ec_point_format_list", nil), "ec_point_format_list"},
		{"compressed points", mutateExtension("ec_point_formats", "ec_point_format_list", []string{"ansiX962_compressed_prime"}), "uncompressed"},
		{"duplicate points", mutateExtension("ec_point_formats", "ec_point_format_list", []string{"uncompressed", "uncompressed"}), "1..1"},
		{"numeric point", mutateExtension("ec_point_formats", "ec_point_format_list", []any{0}), "ec_point_format_list[0]"},
		{"null signatures", mutateExtension("signature_algorithms", "supported_signature_algorithms", nil), "supported_signature_algorithms"},
		{"empty signatures", mutateExtension("signature_algorithms", "supported_signature_algorithms", []string{}), "supported_signature_algorithms"},
		{"too many signatures", mutateExtension("signature_algorithms", "supported_signature_algorithms", make([]any, 33)), "1..32"},
		{"duplicate signatures", mutateExtension("signature_algorithms", "supported_signature_algorithms", []string{"ed25519", "ed25519"}), "duplicate entry"},
		{"unknown signature", mutateExtension("signature_algorithms", "supported_signature_algorithms", []string{"private-content-value"}), "unsupported signature scheme"},
		{"legacy only signatures", mutateExtension("signature_algorithms", "supported_signature_algorithms", []string{"ecdsa_sha1", "rsa_pkcs1_sha1"}), "modern signature scheme"},
		{"pkcs1 only tls13 signatures", mutateExtension("signature_algorithms", "supported_signature_algorithms", []string{"rsa_pkcs1_sha256"}), "TLS 1.3 requires"},
		{"rsa suites with ec signatures", func(r map[string]any) {
			r["cipher_suites"] = []string{"TLS_AES_128_GCM_SHA256", "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"}
			recipeExtension(t, r, "signature_algorithms")["supported_signature_algorithms"] = []string{"ecdsa_secp256r1_sha256"}
		}, "compatible with signature_algorithms"},
		{"ec suites with rsa signatures", func(r map[string]any) {
			r["cipher_suites"] = []string{"TLS_AES_128_GCM_SHA256", "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256"}
			recipeExtension(t, r, "signature_algorithms")["supported_signature_algorithms"] = []string{"rsa_pss_rsae_sha256"}
		}, "compatible with signature_algorithms"},
		{"null alpn", mutateExtension("application_layer_protocol_negotiation", "protocol_name_list", nil), "protocol_name_list"},
		{"empty alpn", mutateExtension("application_layer_protocol_negotiation", "protocol_name_list", []string{}), "protocol_name_list"},
		{"h2 only", mutateExtension("application_layer_protocol_negotiation", "protocol_name_list", []string{"h2"}), "h2-only"},
		{"duplicate alpn", mutateExtension("application_layer_protocol_negotiation", "protocol_name_list", []string{"http/1.1", "http/1.1"}), "duplicate entry"},
		{"empty protocol", mutateExtension("application_layer_protocol_negotiation", "protocol_name_list", []string{""}), "protocol_name_list[0]"},
		{"unknown alpn", mutateExtension("application_layer_protocol_negotiation", "protocol_name_list", []string{"h3", "http/1.1"}), "unsupported ALPN"},
		{"null versions", mutateExtension("supported_versions", "versions", nil), "versions"},
		{"empty versions", mutateExtension("supported_versions", "versions", []string{}), "versions"},
		{"too many versions", mutateExtension("supported_versions", "versions", []string{"GREASE", "TLS 1.3", "TLS 1.2", "TLS 1.1"}), "1..3"},
		{"old version", mutateExtension("supported_versions", "versions", []string{"TLS 1.1"}), "unsupported TLS version"},
		{"numeric version", mutateExtension("supported_versions", "versions", []any{772}), "versions[0]"},
		{"duplicate version", mutateExtension("supported_versions", "versions", []string{"TLS 1.3", "TLS 1.3"}), "duplicate entry"},
		{"only grease version", mutateExtension("supported_versions", "versions", []string{"GREASE"}), "real TLS version"},
		{"null min", func(r map[string]any) { r["min_vers"] = nil }, "min_vers"},
		{"string min", func(r map[string]any) { r["min_vers"] = "771" }, "min_vers"},
		{"old min", func(r map[string]any) { r["min_vers"] = 770 }, "min_vers"},
		{"contradictory min", func(r map[string]any) { r["min_vers"] = 772 }, "contradicts"},
		{"contradictory max", func(r map[string]any) { r["max_vers"] = 771 }, "contradicts"},
		{"inverted bounds", func(r map[string]any) { r["min_vers"], r["max_vers"] = 772, 771 }, "contradicts"},
		{"overflow max", func(r map[string]any) { r["max_vers"] = json.Number("18446744073709551616") }, "max_vers"},
		{"fractional min", func(r map[string]any) { r["min_vers"] = json.Number("771.0") }, "min_vers"},
		{"exponent max", func(r map[string]any) { r["max_vers"] = json.Number("7.72e2") }, "max_vers"},
		{"null shares", mutateExtension("key_share", "client_shares", nil), "client_shares"},
		{"empty shares", mutateExtension("key_share", "client_shares", []any{}), "client_shares"},
		{"too many shares", mutateExtension("key_share", "client_shares", make([]any, 4)), "1..3"},
		{"null share", mutateExtension("key_share", "client_shares", []any{nil}), "client_shares[0]"},
		{"unknown share", mutateExtension("key_share", "client_shares", []any{share("private-content-value")}), "unsupported key-share group"},
		{"numeric share", mutateExtension("key_share", "client_shares", []any{map[string]any{"group": 29}}), "client_shares[0].group"},
		{"fixed key", mutateExtension("key_share", "client_shares", []any{share("x25519", []int{0})}), "key_exchange: must be omitted"},
		{"empty fixed key", mutateExtension("key_share", "client_shares", []any{share("x25519", []int{})}), "key_exchange: must be omitted"},
		{"null fixed key", mutateExtension("key_share", "client_shares", []any{share("x25519", nil)}), "key_exchange: must be omitted"},
		{"duplicate shares", mutateExtension("key_share", "client_shares", []any{share("x25519"), share("x25519")}), "duplicate entry"},
		{"mlkem only", mutateExtension("key_share", "client_shares", []any{share("X25519MLKEM768")}), "classical share"},
		{"share not in groups", mutateExtension("key_share", "client_shares", []any{share("secp521r1")}), "listed in supported_groups"},
		{"share order", mutateExtension("key_share", "client_shares", []any{share("x25519"), share("X25519MLKEM768")}), "supported_groups order"},
		{"multiple classical shares", mutateExtension("key_share", "client_shares", []any{share("x25519"), share("secp256r1")}), "one real classical"},
		{"wrong mlkem fallback", mutateExtension("key_share", "client_shares", []any{share("X25519MLKEM768"), share("secp256r1")}), "x25519 fallback"},
		{"no key share for tls13", func(r map[string]any) { removeExtensions(r, "key_share") }, "TLS 1.3 requires key_share"},
		{"no versions for tls13 bounds", func(r map[string]any) { removeExtensions(r, "supported_versions") }, "max_vers: contradicts"},
		{"null psk modes", mutateExtension("psk_key_exchange_modes", "ke_modes", nil), "ke_modes"},
		{"empty psk modes", mutateExtension("psk_key_exchange_modes", "ke_modes", []string{}), "ke_modes"},
		{"plain psk", mutateExtension("psk_key_exchange_modes", "ke_modes", []string{"psk_ke"}), "only psk_dhe_ke"},
		{"extra psk mode", mutateExtension("psk_key_exchange_modes", "ke_modes", []string{"psk_dhe_ke", "psk_dhe_ke"}), "1..1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recipe := fixtureRecipe(t)
			tc.mutate(recipe)
			_, err := ParseCustom(recipeData(t, recipe))
			if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "private-content-value") {
				t.Fatalf("error = %v, want %q without arbitrary content", err, tc.want)
			}
		})
	}
}

func TestUnsupportedRecipeSymbolsAndPayloads(t *testing.T) {
	for _, name := range []string{"application_settings", "application_settings_new", "compress_certificate", "record_size_limit", "delegated_credentials", "encrypted_client_hello", "next_protocol_negotiation", "quic_transport_parameters", "cookie", "pre_shared_key", "early_data", "token_binding", "generic", "raw", "private-content-value", "Server_Name"} {
		recipe := fixtureRecipe(t)
		recipeExtension(t, recipe, "signed_certificate_timestamp")["name"] = name
		if _, err := ParseCustom(recipeData(t, recipe)); err == nil || !strings.Contains(err.Error(), "extensions[9].name") || strings.Contains(err.Error(), "private-content-value") {
			t.Fatalf("unsupported extension %s: %v", name, err)
		}
	}
	for _, name := range []string{"TLS_RSA_WITH_3DES_EDE_CBC_SHA", "TLS_RSA_WITH_RC4_128_SHA", "TLS_RSA_WITH_NULL_SHA", "TLS_AES_128_CCM_SHA256", "TLS_DHE_RSA_WITH_AES_128_GCM_SHA256", "TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305", "tls_aes_128_gcm_sha256"} {
		recipe := fixtureRecipe(t)
		recipe["cipher_suites"].([]any)[0] = name
		if _, err := ParseCustom(recipeData(t, recipe)); err == nil {
			t.Fatalf("unsupported suite %s accepted", name)
		}
	}
	for _, name := range []string{"X25519", "x25519_kyber768", "X25519Kyber768Draft00", "ffdhe2048", "secp192r1", "brainpoolP256r1"} {
		recipe := fixtureRecipe(t)
		recipeExtension(t, recipe, "supported_groups")["named_group_list"] = []string{name}
		if _, err := ParseCustom(recipeData(t, recipe)); err == nil {
			t.Fatalf("unsupported group %s accepted", name)
		}
	}
	for _, name := range []string{"dsa_sha256", "rsa_pkcs1_sha224", "ed448", "rsa_pss_pss_sha256", "rsa_pkcs1_sha256_legacy", "ecdsa_brainpoolP256r1tls13_sha256", "GREASE"} {
		recipe := fixtureRecipe(t)
		recipeExtension(t, recipe, "signature_algorithms")["supported_signature_algorithms"] = []string{name}
		if _, err := ParseCustom(recipeData(t, recipe)); err == nil {
			t.Fatalf("unsupported signature %s accepted", name)
		}
	}
	for _, name := range []string{"server_name", "extended_master_secret", "renegotiation_info", "session_ticket", "status_request", "signed_certificate_timestamp", "supported_groups", "ec_point_formats", "application_layer_protocol_negotiation", "signature_algorithms", "key_share", "psk_key_exchange_modes", "supported_versions"} {
		recipe := fixtureRecipe(t)
		recipeExtension(t, recipe, name)["private-content-value"] = "private-content-value"
		if _, err := ParseCustom(recipeData(t, recipe)); err == nil || !strings.Contains(err.Error(), "unknown field") || strings.Contains(err.Error(), "private-content-value") {
			t.Fatalf("unsupported payload on %s: %v", name, err)
		}
	}
}

func TestAcceptedCustomVariations(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(map[string]any)
		min    uint16
		max    uint16
	}{
		{"no alpn", func(r map[string]any) { removeExtensions(r, "application_layer_protocol_negotiation") }, 771, 772},
		{"h2 first", func(r map[string]any) {
			recipeExtension(t, r, "application_layer_protocol_negotiation")["protocol_name_list"] = []string{"h2", "http/1.1"}
		}, 771, 772},
		{"h1 first", func(r map[string]any) {
			recipeExtension(t, r, "application_layer_protocol_negotiation")["protocol_name_list"] = []string{"http/1.1", "h2"}
		}, 771, 772},
		{"tls12 inferred", func(r map[string]any) {
			removeExtensions(r, "supported_versions", "key_share", "psk_key_exchange_modes")
			delete(r, "min_vers")
			delete(r, "max_vers")
		}, 771, 771},
		{"tls12 advertised", func(r map[string]any) {
			removeExtensions(r, "key_share", "psk_key_exchange_modes")
			recipeExtension(t, r, "supported_versions")["versions"] = []string{"TLS 1.2"}
			r["max_vers"] = 771
		}, 771, 771},
		{"tls13 only", func(r map[string]any) {
			recipeExtension(t, r, "supported_versions")["versions"] = []string{"TLS 1.3"}
			r["min_vers"] = 772
			r["cipher_suites"] = []string{"TLS_AES_128_GCM_SHA256"}
		}, 772, 772},
		{"reverse versions", func(r map[string]any) {
			recipeExtension(t, r, "supported_versions")["versions"] = []string{"TLS 1.2", "TLS 1.3"}
		}, 771, 772},
		{"all modern and measured signatures", func(r map[string]any) {
			recipeExtension(t, r, "signature_algorithms")["supported_signature_algorithms"] = []string{"ecdsa_secp256r1_sha256", "ecdsa_secp384r1_sha384", "ecdsa_secp521r1_sha512", "rsa_pss_rsae_sha256", "rsa_pss_rsae_sha384", "rsa_pss_rsae_sha512", "rsa_pkcs1_sha256", "rsa_pkcs1_sha384", "rsa_pkcs1_sha512", "ed25519", "rsa_pkcs1_sha1", "ecdsa_sha1"}
		}, 771, 772},
		{"minimal tls12", func(r map[string]any) {
			r["cipher_suites"] = []string{"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"}
			r["extensions"] = []any{map[string]any{"name": "server_name"}, map[string]any{"name": "supported_groups", "named_group_list": []string{"x25519"}}, map[string]any{"name": "signature_algorithms", "supported_signature_algorithms": []string{"rsa_pkcs1_sha256"}}}
			delete(r, "min_vers")
			delete(r, "max_vers")
		}, 771, 771},
		{"tls12 ed25519 auth", func(r map[string]any) {
			r["cipher_suites"] = []string{"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256"}
			r["extensions"] = []any{map[string]any{"name": "server_name"}, map[string]any{"name": "supported_groups", "named_group_list": []string{"x25519"}}, map[string]any{"name": "signature_algorithms", "supported_signature_algorithms": []string{"ed25519"}}}
			delete(r, "min_vers")
			delete(r, "max_vers")
		}, 771, 771},
	}
	for _, group := range []string{"x25519", "secp256r1", "secp384r1", "secp521r1"} {
		cases = append(cases, struct {
			name   string
			mutate func(map[string]any)
			min    uint16
			max    uint16
		}{group + " share", func(r map[string]any) {
			recipeExtension(t, r, "supported_groups")["named_group_list"] = []string{group}
			recipeExtension(t, r, "key_share")["client_shares"] = []any{map[string]any{"group": group}}
		}, 771, 772})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recipe := fixtureRecipe(t)
			tc.mutate(recipe)
			spec, err := DecodeSpec(recipeData(t, recipe))
			if err != nil || spec.TLSVersMin != tc.min || spec.TLSVersMax != tc.max {
				t.Fatalf("accepted recipe: %v", err)
			}
		})
	}
	for _, name := range []string{"key_share", "psk_key_exchange_modes"} {
		recipe := fixtureRecipe(t)
		removeExtensions(recipe, "key_share", "psk_key_exchange_modes")
		recipeExtension(t, recipe, "supported_versions")["versions"] = []string{"TLS 1.2"}
		recipe["max_vers"] = 771
		var extension map[string]any
		if name == "key_share" {
			extension = map[string]any{"name": name, "client_shares": []any{map[string]any{"group": "x25519"}}}
		} else {
			extension = map[string]any{"name": name, "ke_modes": []string{"psk_dhe_ke"}}
		}
		recipe["extensions"] = append(recipe["extensions"].([]any), extension)
		if _, err := ParseCustom(recipeData(t, recipe)); err == nil || !strings.Contains(err.Error(), "requires TLS 1.3") {
			t.Fatalf("contradictory TLS-1.3-only extension: %v", err)
		}
	}
}

func greaseRecipe(t *testing.T, keyExchange ...any) map[string]any {
	t.Helper()
	recipe := fixtureRecipe(t)
	recipe["cipher_suites"] = append([]any{"GREASE"}, recipe["cipher_suites"].([]any)...)
	recipeExtension(t, recipe, "supported_groups")["named_group_list"] = []string{"GREASE", "X25519MLKEM768", "x25519", "secp256r1"}
	grease := map[string]any{"group": "GREASE"}
	if len(keyExchange) > 0 {
		grease["key_exchange"] = keyExchange[0]
	}
	recipeExtension(t, recipe, "key_share")["client_shares"] = []any{grease, map[string]any{"group": "X25519MLKEM768"}, map[string]any{"group": "x25519"}}
	recipeExtension(t, recipe, "supported_versions")["versions"] = []string{"GREASE", "TLS 1.3", "TLS 1.2"}
	recipe["extensions"] = append(append([]any{map[string]any{"name": "GREASE"}}, recipe["extensions"].([]any)...), map[string]any{"name": "GREASE"})
	return recipe
}

func TestGreaseSubset(t *testing.T) {
	for _, recipe := range []map[string]any{greaseRecipe(t), greaseRecipe(t, []int{0})} {
		profile, err := ParseCustom(recipeData(t, recipe))
		if err != nil {
			t.Fatal(err)
		}
		spec := materialize(t, profile)
		for _, extension := range spec.Extensions {
			if shares, ok := extension.(*utls.KeyShareExtension); ok && !bytes.Equal(shares.KeyShares[0].Data, []byte{0}) {
				t.Fatal("GREASE share must use canonical dummy [0]")
			}
		}
		connection := buildHello(t, spec, "clienthello.invalid")
		first := connection.Extensions[0].(*utls.UtlsGREASEExtension)
		last := connection.Extensions[len(connection.Extensions)-1].(*utls.UtlsGREASEExtension)
		if first.Value == last.Value || len(first.Body) != 0 || !bytes.Equal(last.Body, []byte{0}) {
			t.Fatal("uTLS GREASE extension behavior changed")
		}
	}
	for _, value := range []any{nil, []int{}, []int{1}, []int{0, 0}, "AA==", []any{nil}, []any{-1}, []any{json.Number("0.0")}} {
		if _, err := ParseCustom(recipeData(t, greaseRecipe(t, value))); err == nil {
			t.Fatalf("invalid GREASE exchange accepted: %T", value)
		}
	}
	for _, mutate := range []func(map[string]any){
		func(r map[string]any) {
			r["extensions"] = append(r["extensions"].([]any), map[string]any{"name": "GREASE"})
		},
		func(r map[string]any) { r["extensions"].([]any)[0].(map[string]any)["id"] = 0x0a0a },
		func(r map[string]any) { r["extensions"].([]any)[0].(map[string]any)["data"] = "AA==" },
		func(r map[string]any) {
			recipeExtension(t, r, "key_share")["client_shares"] = []any{map[string]any{"group": "GREASE"}}
		},
		func(r map[string]any) {
			recipeExtension(t, r, "supported_groups")["named_group_list"] = []string{"X25519MLKEM768", "x25519"}
		},
	} {
		recipe := greaseRecipe(t)
		mutate(recipe)
		if _, err := ParseCustom(recipeData(t, recipe)); err == nil {
			t.Fatal("unsupported GREASE shape accepted")
		}
	}
}

func TestPaddingAndClientHelloBounds(t *testing.T) {
	for _, length := range []any{0, 1, 4096, nil, -1, 4097, json.Number("4294967296"), json.Number("0.5"), "0"} {
		t.Run(fmt.Sprint(length), func(t *testing.T) {
			recipe := fixtureRecipe(t)
			recipe["extensions"] = append(recipe["extensions"].([]any), map[string]any{"name": "padding", "len": length})
			profile, err := ParseCustom(recipeData(t, recipe))
			valid := length == 0 || length == 1 || length == 4096
			if (err == nil) != valid {
				t.Fatalf("padding %v: %v", length, err)
			}
			if !valid {
				return
			}
			spec := materialize(t, profile)
			padding := spec.Extensions[len(spec.Extensions)-1].(*utls.UtlsPaddingExtension)
			if length == 0 {
				if padding.GetPaddingLen == nil || padding.WillPad {
					t.Fatal("zero padding must select BoringPaddingStyle")
				}
			} else if padding.GetPaddingLen != nil || !padding.WillPad || padding.PaddingLen != length {
				t.Fatal("fixed padding changed")
			}
		})
	}
	recipe := fixtureRecipe(t)
	recipe["extensions"] = append(recipe["extensions"].([]any), map[string]any{"name": "padding"})
	if _, err := ParseCustom(recipeData(t, recipe)); err == nil {
		t.Fatal("padding without explicit len must be rejected")
	}
	profile, err := ParseCustom(fixtureData(t))
	if err != nil {
		t.Fatal(err)
	}
	baseLength := len(buildHello(t, materialize(t, profile), "clienthello.invalid").HandshakeState.Hello.Raw)
	for _, total := range []int{MaxClientHelloBytes, MaxClientHelloBytes + 1} {
		spec := materialize(t, profile)
		spec.Extensions = append(spec.Extensions, &utls.UtlsPaddingExtension{PaddingLen: total - baseLength - 4, WillPad: true})
		err := validateClientHelloSize(spec)
		if (err == nil) != (total == MaxClientHelloBytes) {
			t.Fatalf("dry-run boundary %d: %v", total, err)
		}
	}
}

func FuzzParseCustom(f *testing.F) {
	f.Add(fixtureData(f))
	for _, data := range []string{`{}`, `null`, `[]`, `{"extensions":[{"name":"server_name","name":"server_name"}]}`, `{"cipher_suites":null}`, `{"unknown":[[[[[[[[[0]]]]]]]]]}`} {
		f.Add([]byte(data))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		profile, err := ParseCustom(data)
		if err != nil {
			return
		}
		if !profile.IsCustom() || profile.IsNative() {
			t.Fatal("parser returned a noncustom descriptor")
		}
		canonical := profile.CanonicalJSON()
		reparsed, err := ParseCustom(canonical)
		if err != nil || reparsed.Identity() != profile.Identity() {
			t.Fatalf("canonical recipe cannot be replayed: %v", err)
		}
		if _, err := profile.Materialize(); err != nil {
			t.Fatalf("validated profile cannot be materialized: %v", err)
		}
	})
}
