package tlsprofile

import (
	"encoding/json"
	"fmt"
	"strings"

	utls "github.com/refraction-networking/utls"
)

const (
	version12Mask = 1 << iota
	version13Mask
)

type recipeFeatures struct {
	extensions map[string]int
	groups     []string
	shares     []string
	versions   []string
	suites     []string
	signatures []string
	safeSuites int
	modernSigs bool
	tls13Sigs  bool
}

func validateRecipe(data []byte) ([]byte, error) {
	root, err := readRecipe(data)
	if err != nil {
		return nil, err
	}
	if _, err := recipeObject(root, "spec", "cipher_suites", "compression_methods", "extensions", "min_vers", "max_vers"); err != nil {
		return nil, err
	}
	suites, err := recipeStrings(root["cipher_suites"], "cipher_suites", 1, 64)
	if err != nil {
		return nil, err
	}
	if err := recipeUnique(suites, "cipher_suites"); err != nil {
		return nil, err
	}
	features := recipeFeatures{extensions: make(map[string]int), suites: suites}
	for i, suite := range suites {
		versions, allowed := cipherVersions(suite)
		if !allowed {
			return nil, recipeError(recipeIndex("cipher_suites", i), "unsupported cipher suite")
		}
		features.safeSuites |= versions
	}
	compression, err := recipeStrings(root["compression_methods"], "compression_methods", 1, 1)
	if err != nil {
		return nil, err
	}
	if compression[0] != "NULL" {
		return nil, recipeError("compression_methods[0]", "only NULL compression is supported")
	}
	extensions, err := recipeArray(root["extensions"], "extensions", 1, 32)
	if err != nil {
		return nil, err
	}
	greaseCount := 0
	for i, extension := range extensions {
		field := recipeIndex("extensions", i)
		object, ok := extension.(map[string]any)
		if !ok {
			return nil, recipeError(field, "must be an object")
		}
		name, ok := object["name"].(string)
		if !ok || name == "" {
			return nil, recipeError(field+".name", "must be a nonempty extension name")
		}
		if name == "GREASE" {
			greaseCount++
			if greaseCount > 2 {
				return nil, recipeError(field+".name", "at most two GREASE extensions are supported")
			}
		} else {
			if _, exists := features.extensions[name]; exists {
				return nil, recipeError(field+".name", "duplicate extension")
			}
		}
		features.extensions[name] = i
		if err := validateExtension(object, name, field, &features); err != nil {
			return nil, err
		}
	}
	for _, name := range []string{"server_name", "supported_groups", "signature_algorithms"} {
		if _, ok := features.extensions[name]; !ok {
			return nil, recipeError("extensions", "requires "+name)
		}
	}
	min, max, enabled, err := recipeVersions(root, features.versions)
	if err != nil {
		return nil, err
	}
	if enabled&features.safeSuites != enabled {
		return nil, recipeError("cipher_suites", "requires a safe usable suite for each enabled TLS version")
	}
	if !features.modernSigs {
		return nil, recipeError(features.field("signature_algorithms", "supported_signature_algorithms"), "requires a supported modern signature scheme")
	}
	if enabled&version12Mask != 0 && !usableTLS12Suite(features.suites, features.signatures) {
		return nil, recipeError("cipher_suites", "TLS 1.2 requires a safe suite compatible with signature_algorithms")
	}
	if enabled&version12Mask != 0 && !hasClassicalGroup(features.groups) {
		return nil, recipeError(features.field("supported_groups", "named_group_list"), "TLS 1.2 requires a supported classical group")
	}
	if enabled&version13Mask != 0 {
		if !features.tls13Sigs {
			return nil, recipeError(features.field("signature_algorithms", "supported_signature_algorithms"), "TLS 1.3 requires ECDSA, RSA-PSS, or Ed25519 signatures")
		}
		if _, ok := features.extensions["key_share"]; !ok {
			return nil, recipeError("extensions", "TLS 1.3 requires key_share")
		}
	} else {
		for _, name := range []string{"key_share", "psk_key_exchange_modes"} {
			if _, ok := features.extensions[name]; ok {
				return nil, recipeError(features.field(name, ""), "requires TLS 1.3 in supported_versions")
			}
		}
	}
	if err := validateShares(features); err != nil {
		return nil, err
	}
	root["min_vers"] = min
	root["max_vers"] = max
	canonical, err := json.Marshal(root)
	if err != nil {
		return nil, recipeError("spec", "cannot canonicalize JSON")
	}
	return canonical, nil
}

func (f recipeFeatures) field(name, child string) string {
	field := recipeIndex("extensions", f.extensions[name])
	if child != "" {
		field += "." + child
	}
	return field
}

func validateExtension(object map[string]any, name, field string, features *recipeFeatures) error {
	switch name {
	case "GREASE", "server_name", "extended_master_secret", "renegotiation_info", "session_ticket", "status_request", "signed_certificate_timestamp":
		_, err := recipeObject(object, field, "name")
		return err
	case "supported_groups":
		if _, err := recipeObject(object, field, "name", "named_group_list"); err != nil {
			return err
		}
		groups, err := recipeStrings(object["named_group_list"], field+".named_group_list", 1, 16)
		if err != nil {
			return err
		}
		if err := recipeUnique(groups, field+".named_group_list"); err != nil {
			return err
		}
		real := false
		for i, group := range groups {
			if _, ok := groupID(group); !ok {
				return recipeError(recipeIndex(field+".named_group_list", i), "unsupported named group")
			}
			real = real || group != "GREASE"
		}
		if !real {
			return recipeError(field+".named_group_list", "requires a real supported group")
		}
		features.groups = groups
	case "ec_point_formats":
		if _, err := recipeObject(object, field, "name", "ec_point_format_list"); err != nil {
			return err
		}
		points, err := recipeStrings(object["ec_point_format_list"], field+".ec_point_format_list", 1, 1)
		if err != nil {
			return err
		}
		if points[0] != "uncompressed" {
			return recipeError(field+".ec_point_format_list[0]", "only uncompressed points are supported")
		}
	case "signature_algorithms":
		if _, err := recipeObject(object, field, "name", "supported_signature_algorithms"); err != nil {
			return err
		}
		signatures, err := recipeStrings(object["supported_signature_algorithms"], field+".supported_signature_algorithms", 1, 32)
		if err != nil {
			return err
		}
		if err := recipeUnique(signatures, field+".supported_signature_algorithms"); err != nil {
			return err
		}
		features.signatures = signatures
		for i, signature := range signatures {
			versions, allowed := signatureVersions(signature)
			if !allowed {
				return recipeError(recipeIndex(field+".supported_signature_algorithms", i), "unsupported signature scheme")
			}
			features.modernSigs = features.modernSigs || versions != 0
			features.tls13Sigs = features.tls13Sigs || versions&version13Mask != 0
		}
	case "application_layer_protocol_negotiation":
		if _, err := recipeObject(object, field, "name", "protocol_name_list"); err != nil {
			return err
		}
		protocols, err := recipeStrings(object["protocol_name_list"], field+".protocol_name_list", 1, 2)
		if err != nil {
			return err
		}
		if err := recipeUnique(protocols, field+".protocol_name_list"); err != nil {
			return err
		}
		h1 := false
		for i, protocol := range protocols {
			if protocol != "h2" && protocol != "http/1.1" {
				return recipeError(recipeIndex(field+".protocol_name_list", i), "unsupported ALPN protocol")
			}
			h1 = h1 || protocol == "http/1.1"
		}
		if !h1 {
			return recipeError(field+".protocol_name_list", "h2-only profiles are not supported")
		}
	case "supported_versions":
		if _, err := recipeObject(object, field, "name", "versions"); err != nil {
			return err
		}
		versions, err := recipeStrings(object["versions"], field+".versions", 1, 3)
		if err != nil {
			return err
		}
		if err := recipeUnique(versions, field+".versions"); err != nil {
			return err
		}
		for i, version := range versions {
			if version != "TLS 1.2" && version != "TLS 1.3" && version != "GREASE" {
				return recipeError(recipeIndex(field+".versions", i), "unsupported TLS version")
			}
		}
		features.versions = versions
	case "key_share":
		if _, err := recipeObject(object, field, "name", "client_shares"); err != nil {
			return err
		}
		shares, err := recipeArray(object["client_shares"], field+".client_shares", 1, 3)
		if err != nil {
			return err
		}
		for i, entry := range shares {
			shareField := recipeIndex(field+".client_shares", i)
			share, err := recipeObject(entry, shareField, "group", "key_exchange")
			if err != nil {
				return err
			}
			group, ok := share["group"].(string)
			if !ok || group == "" {
				return recipeError(shareField+".group", "must be a nonempty group name")
			}
			if _, ok := groupID(group); !ok {
				return recipeError(shareField+".group", "unsupported key-share group")
			}
			if data, exists := share["key_exchange"]; exists {
				if group != "GREASE" {
					return recipeError(shareField+".key_exchange", "must be omitted for real groups")
				}
				array, err := recipeArray(data, shareField+".key_exchange", 1, 1)
				if err != nil {
					return err
				}
				if _, err := recipeInteger(array[0], shareField+".key_exchange[0]", 0); err != nil {
					return err
				}
			}
			if group == "GREASE" {
				share["key_exchange"] = []any{0}
			}
			features.shares = append(features.shares, group)
		}
		if err := recipeUnique(features.shares, field+".client_shares"); err != nil {
			return err
		}
	case "psk_key_exchange_modes":
		if _, err := recipeObject(object, field, "name", "ke_modes"); err != nil {
			return err
		}
		modes, err := recipeStrings(object["ke_modes"], field+".ke_modes", 1, 1)
		if err != nil {
			return err
		}
		if modes[0] != "psk_dhe_ke" {
			return recipeError(field+".ke_modes[0]", "only psk_dhe_ke is supported")
		}
	case "padding":
		if _, err := recipeObject(object, field, "name", "len"); err != nil {
			return err
		}
		if _, err := recipeInteger(object["len"], field+".len", 4096); err != nil {
			return err
		}
	default:
		return recipeError(field+".name", "unsupported extension")
	}
	return nil
}

func recipeVersions(root map[string]any, versions []string) (uint16, uint16, int, error) {
	min, max := uint16(utls.VersionTLS12), uint16(utls.VersionTLS12)
	enabled := 0
	for _, version := range versions {
		switch version {
		case "TLS 1.2":
			enabled |= version12Mask
		case "TLS 1.3":
			enabled |= version13Mask
		}
	}
	if len(versions) == 0 {
		enabled = version12Mask
	} else if enabled == 0 {
		return 0, 0, 0, recipeError("extensions", "supported_versions requires a real TLS version")
	}
	if enabled&version12Mask == 0 {
		min = utls.VersionTLS13
	}
	if enabled&version13Mask != 0 {
		max = utls.VersionTLS13
	}
	for _, bound := range []struct {
		name string
		want uint16
	}{{"min_vers", min}, {"max_vers", max}} {
		if value, exists := root[bound.name]; exists {
			number, err := recipeInteger(value, bound.name, utls.VersionTLS13)
			if err != nil {
				return 0, 0, 0, err
			}
			if number != utls.VersionTLS12 && number != utls.VersionTLS13 {
				return 0, 0, 0, recipeError(bound.name, "must be 771 or 772")
			}
			if uint16(number) != bound.want {
				return 0, 0, 0, recipeError(bound.name, "contradicts supported_versions (without it, only TLS 1.2 is supported)")
			}
		}
	}
	return min, max, enabled, nil
}

func validateShares(features recipeFeatures) error {
	if len(features.shares) == 0 {
		return nil
	}
	field := features.field("key_share", "client_shares")
	classical, mlkem, x25519 := 0, false, false
	lastGroup := -1
	for i, share := range features.shares {
		groupIndex := -1
		for index, group := range features.groups {
			if group == share {
				groupIndex = index
				break
			}
		}
		if groupIndex == -1 {
			return recipeError(recipeIndex(field, i)+".group", "must be listed in supported_groups")
		}
		if groupIndex <= lastGroup {
			return recipeError(recipeIndex(field, i)+".group", "must follow supported_groups order")
		}
		lastGroup = groupIndex
		switch share {
		case "GREASE":
		case "X25519MLKEM768":
			mlkem = true
		default:
			classical++
			x25519 = x25519 || share == "x25519"
			if classical > 1 {
				return recipeError(recipeIndex(field, i)+".group", "at most one real classical key share is supported")
			}
		}
	}
	if classical == 0 || mlkem && !x25519 {
		return recipeError(field, "requires one classical share; X25519MLKEM768 requires x25519 fallback")
	}
	return nil
}

func hasClassicalGroup(groups []string) bool {
	for _, group := range groups {
		if group != "GREASE" && group != "X25519MLKEM768" {
			return true
		}
	}
	return false
}

func cipherVersions(name string) (int, bool) {
	switch name {
	case "TLS_AES_128_GCM_SHA256", "TLS_AES_256_GCM_SHA384", "TLS_CHACHA20_POLY1305_SHA256":
		return version13Mask, true
	case "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256", "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256", "TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384", "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384", "TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256", "TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256":
		return version12Mask, true
	case "GREASE", "TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA", "TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA", "TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA", "TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA", "TLS_RSA_WITH_AES_128_GCM_SHA256", "TLS_RSA_WITH_AES_256_GCM_SHA384", "TLS_RSA_WITH_AES_128_CBC_SHA", "TLS_RSA_WITH_AES_256_CBC_SHA":
		return 0, true
	default:
		return 0, false
	}
}

func signatureVersions(name string) (int, bool) {
	switch name {
	case "ecdsa_secp256r1_sha256", "ecdsa_secp384r1_sha384", "ecdsa_secp521r1_sha512", "rsa_pss_rsae_sha256", "rsa_pss_rsae_sha384", "rsa_pss_rsae_sha512", "ed25519":
		return version12Mask | version13Mask, true
	case "rsa_pkcs1_sha256", "rsa_pkcs1_sha384", "rsa_pkcs1_sha512":
		return version12Mask, true
	case "rsa_pkcs1_sha1", "ecdsa_sha1":
		return 0, true
	default:
		return 0, false
	}
}

func usableTLS12Suite(suites, signatures []string) bool {
	rsa, ec := false, false
	for _, signature := range signatures {
		versions, _ := signatureVersions(signature)
		if versions&version12Mask == 0 {
			continue
		}
		if strings.HasPrefix(signature, "rsa_") {
			rsa = true
		} else {
			ec = true
		}
	}
	for _, suite := range suites {
		versions, _ := cipherVersions(suite)
		if versions&version12Mask == 0 {
			continue
		}
		if strings.HasPrefix(suite, "TLS_ECDHE_RSA_") {
			if rsa {
				return true
			}
		} else if ec {
			return true
		}
	}
	return false
}

func validateClientHelloSize(spec *utls.ClientHelloSpec) (err error) {
	defer func() {
		if recover() != nil {
			err = recipeError("spec", "cannot build a supported ClientHello")
		}
	}()
	connection := utls.UClient(nil, &utls.Config{ServerName: "clienthello.invalid", MinVersion: utls.VersionTLS12, MaxVersion: utls.VersionTLS13}, utls.HelloCustom)
	if err := connection.ApplyPreset(spec); err != nil {
		return recipeError("spec", "cannot apply a supported ClientHello")
	}
	if err := connection.BuildHandshakeState(); err != nil {
		return recipeError("spec", "cannot build a supported ClientHello")
	}
	if len(connection.HandshakeState.Hello.Raw) > MaxClientHelloBytes {
		return recipeError("spec", fmt.Sprintf("ClientHello must be at most %d bytes", MaxClientHelloBytes))
	}
	return nil
}
