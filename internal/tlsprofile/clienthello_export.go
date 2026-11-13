package tlsprofile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"unicode/utf8"

	utls "github.com/refraction-networking/utls"
	"github.com/refraction-networking/utls/dicttls"
)

func ExportClientHello(data []byte) (recipe []byte, err error) {
	defer func() {
		if recover() != nil {
			recipe = nil
			err = fmt.Errorf("ClientHello cannot be exported as a supported profile")
		}
	}()
	hello, err := parseCapturedHello(data)
	if err != nil {
		return nil, err
	}
	if err := validateCapturedHello(hello, false); err != nil {
		return nil, err
	}
	spec, err := fingerprintCapturedHello(hello)
	if err != nil {
		return nil, err
	}
	root, err := exportCapturedSpec(spec, nil)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("cannot encode the ClientHello recipe")
	}
	profile, err := ParseCustom(encoded)
	if err != nil {
		return nil, fmt.Errorf("captured ClientHello is outside the supported custom schema: %w", err)
	}
	var output bytes.Buffer
	if err := json.Indent(&output, profile.CanonicalJSON(), "", "  "); err != nil {
		return nil, fmt.Errorf("cannot format the ClientHello recipe")
	}
	return append(output.Bytes(), '\n'), nil
}

func fingerprintCapturedHello(hello *capturedHello) (spec *utls.ClientHelloSpec, err error) {
	defer func() {
		if recover() != nil {
			spec = nil
			err = fmt.Errorf("ClientHello cannot be fingerprinted with the pinned safe options")
		}
	}()
	version := hello.records[0].version
	record := []byte{22, byte(version >> 8), byte(version), byte(len(hello.handshake) >> 8), byte(len(hello.handshake))}
	record = append(record, hello.handshake...)
	fingerprinter := utls.Fingerprinter{AllowBluntMimicry: false, AlwaysAddPadding: false, RealPSKResumption: false}
	spec, err = fingerprinter.FingerprintClientHello(record)
	if err != nil {
		return nil, fmt.Errorf("ClientHello cannot be fingerprinted with the pinned safe options")
	}
	return spec, nil
}

func validateCapturedSchema(hello *capturedHello) error {
	spec, err := fingerprintCapturedHello(hello)
	if err != nil {
		return err
	}
	var paddingBytes *int
	for _, extension := range hello.extensions {
		if extension.id == 21 {
			length := extension.span.to - extension.span.from
			paddingBytes = &length
		}
	}
	root, err := exportCapturedSpec(spec, paddingBytes)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(root)
	if err != nil {
		return fmt.Errorf("cannot validate the captured ClientHello schema")
	}
	if _, err := validateRecipe(encoded); err != nil {
		return fmt.Errorf("captured ClientHello is outside the supported custom schema: %w", err)
	}
	return nil
}

func validateCapturedHello(hello *capturedHello, allowPadding bool) error {
	if hello.summary.HandshakeVersion != utls.VersionTLS12 {
		return fmt.Errorf("captured ClientHello must use the TLS 1.2 legacy handshake version")
	}
	for _, id := range hello.summary.CipherSuites {
		if _, err := captureCipherName(id); err != nil {
			return err
		}
	}
	if len(hello.summary.CompressionMethods) != 1 || hello.summary.CompressionMethods[0] != 0 {
		return fmt.Errorf("captured ClientHello must use NULL compression only")
	}
	present := make(map[uint16]bool)
	for _, extension := range hello.extensions {
		present[extension.id] = true
		payload := hello.handshake[extension.span.from:extension.span.to]
		switch extension.id {
		case 0:
			if err := validateCapturedSNI(payload); err != nil {
				return err
			}
		case 23, 18, 35:
			if len(payload) != 0 {
				return fmt.Errorf("ClientHello extension %d has unsupported or resumption-dependent data", extension.id)
			}
		case 65281:
			if !bytes.Equal(payload, []byte{0}) {
				return fmt.Errorf("ClientHello renegotiation data must be an empty first-flight advertisement")
			}
		case 10:
			for _, group := range hello.summary.Groups {
				if _, err := captureGroupName(group); err != nil {
					return err
				}
			}
		case 11:
			if !bytes.Equal(payload, []byte{1, 0}) {
				return fmt.Errorf("ClientHello supports only uncompressed EC point formats")
			}
		case 16:
			for _, protocol := range hello.summary.ALPN {
				if protocol != "h2" && protocol != "http/1.1" {
					return fmt.Errorf("ClientHello advertises an unsupported ALPN protocol")
				}
			}
		case 5:
			if !bytes.Equal(payload, []byte{1, 0, 0, 0, 0}) {
				return fmt.Errorf("ClientHello OCSP request contains unsupported data")
			}
		case 13:
			for _, signature := range hello.summary.SignatureAlgorithms {
				if _, err := captureSignatureName(signature); err != nil {
					return err
				}
			}
		case 51:
			for _, share := range hello.keyShares {
				if _, err := captureGroupName(share.group); err != nil {
					return err
				}
				key := hello.handshake[share.span.from:share.span.to]
				if captureGREASE(share.group) {
					if !bytes.Equal(key, []byte{0}) {
						return fmt.Errorf("ClientHello GREASE key exchange must be the canonical dummy byte")
					}
				} else if len(key) != captureKeyBytes(share.group) {
					return fmt.Errorf("ClientHello key exchange has an unsupported group or length")
				}
			}
		case 45:
			if !bytes.Equal(payload, []byte{1, utls.PskModeDHE}) {
				return fmt.Errorf("ClientHello PSK modes must advertise psk_dhe_ke only")
			}
		case 43:
			reader := helloReader{data: payload}
			versions, _, err := reader.vector8("supported versions")
			if err != nil || reader.offset != len(payload) {
				return fmt.Errorf("invalid ClientHello supported versions")
			}
			ids, err := helloUint16List(versions, "supported versions", 3)
			if err != nil {
				return err
			}
			for _, version := range ids {
				if _, err := captureVersionName(version); err != nil {
					return err
				}
			}
		case 21:
			if !allowPadding {
				return fmt.Errorf("padding policy cannot be inferred from a capture; supply a separately validated recipe instead")
			}
			if len(payload) > 4096 || !captureZeroBytes(payload) {
				return fmt.Errorf("ClientHello padding is invalid or oversized")
			}
		default:
			if !captureGREASE(extension.id) {
				return fmt.Errorf("ClientHello extension %d is unsupported; no raw extension export is permitted", extension.id)
			}
			if len(payload) != 0 && !bytes.Equal(payload, []byte{0}) {
				return fmt.Errorf("ClientHello GREASE extension contains unsupported data")
			}
		}
	}
	for _, id := range []uint16{0, 10, 13} {
		if !present[id] {
			return fmt.Errorf("captured ClientHello requires SNI, supported groups, and signatures")
		}
	}
	return nil
}

func validateCapturedSNI(data []byte) error {
	reader := helloReader{data: data}
	list, _, err := reader.vector16("server-name list")
	if err != nil || reader.offset != len(data) {
		return fmt.Errorf("invalid ClientHello server-name list")
	}
	names := helloReader{data: list}
	kind, err := names.take(1, "server-name type")
	if err != nil || kind[0] != 0 {
		return fmt.Errorf("ClientHello requires one host_name SNI entry")
	}
	name, _, err := names.vector16("server name")
	if err != nil || names.offset != len(list) || len(name) == 0 || len(name) > 255 || !utf8.Valid(name) {
		return fmt.Errorf("ClientHello requires one valid server name")
	}
	for _, value := range name {
		if value < 33 || value > 126 {
			return fmt.Errorf("ClientHello server name contains unsupported characters")
		}
	}
	return nil
}

func exportCapturedSpec(spec *utls.ClientHelloSpec, paddingBytes *int) (map[string]any, error) {
	suites := make([]string, 0, len(spec.CipherSuites))
	for _, id := range spec.CipherSuites {
		name, err := captureCipherName(id)
		if err != nil {
			return nil, err
		}
		suites = append(suites, name)
	}
	extensions := make([]map[string]any, 0, len(spec.Extensions))
	for _, extension := range spec.Extensions {
		entry := make(map[string]any)
		switch ext := extension.(type) {
		case *utls.SNIExtension:
			entry["name"] = "server_name"
		case *utls.ExtendedMasterSecretExtension:
			entry["name"] = "extended_master_secret"
		case *utls.RenegotiationInfoExtension:
			entry["name"] = "renegotiation_info"
		case *utls.SessionTicketExtension:
			entry["name"] = "session_ticket"
		case *utls.StatusRequestExtension:
			entry["name"] = "status_request"
		case *utls.SCTExtension:
			entry["name"] = "signed_certificate_timestamp"
		case *utls.UtlsGREASEExtension:
			entry["name"] = "GREASE"
		case *utls.UtlsPaddingExtension:
			if paddingBytes == nil {
				return nil, fmt.Errorf("padding policy cannot be inferred from a capture")
			}
			entry["name"], entry["len"] = "padding", *paddingBytes
		case *utls.SupportedCurvesExtension:
			entry["name"] = "supported_groups"
			groups := make([]string, 0, len(ext.Curves))
			for _, id := range ext.Curves {
				name, err := captureGroupName(uint16(id))
				if err != nil {
					return nil, err
				}
				groups = append(groups, name)
			}
			entry["named_group_list"] = groups
		case *utls.SupportedPointsExtension:
			entry["name"] = "ec_point_formats"
			entry["ec_point_format_list"] = []string{"uncompressed"}
		case *utls.ALPNExtension:
			entry["name"] = "application_layer_protocol_negotiation"
			entry["protocol_name_list"] = ext.AlpnProtocols
		case *utls.SignatureAlgorithmsExtension:
			entry["name"] = "signature_algorithms"
			signatures := make([]string, 0, len(ext.SupportedSignatureAlgorithms))
			for _, id := range ext.SupportedSignatureAlgorithms {
				name, err := captureSignatureName(uint16(id))
				if err != nil {
					return nil, err
				}
				signatures = append(signatures, name)
			}
			entry["supported_signature_algorithms"] = signatures
		case *utls.KeyShareExtension:
			entry["name"] = "key_share"
			shares := make([]map[string]any, 0, len(ext.KeyShares))
			for _, share := range ext.KeyShares {
				name, err := captureGroupName(uint16(share.Group))
				if err != nil {
					return nil, err
				}
				item := map[string]any{"group": name}
				if name == "GREASE" {
					item["key_exchange"] = []int{0}
				}
				shares = append(shares, item)
			}
			entry["client_shares"] = shares
		case *utls.PSKKeyExchangeModesExtension:
			entry["name"] = "psk_key_exchange_modes"
			entry["ke_modes"] = []string{"psk_dhe_ke"}
		case *utls.SupportedVersionsExtension:
			entry["name"] = "supported_versions"
			versions := make([]string, 0, len(ext.Versions))
			for _, id := range ext.Versions {
				name, err := captureVersionName(id)
				if err != nil {
					return nil, err
				}
				versions = append(versions, name)
			}
			entry["versions"] = versions
		default:
			return nil, fmt.Errorf("fingerprinted ClientHello contains an unsupported extension")
		}
		extensions = append(extensions, entry)
	}
	return map[string]any{"cipher_suites": suites, "compression_methods": []string{"NULL"}, "extensions": extensions}, nil
}

func captureGREASE(id uint16) bool {
	return id&0x0f0f == 0x0a0a && byte(id>>8) == byte(id)
}

func captureCipherName(id uint16) (string, error) {
	if captureGREASE(id) {
		return "GREASE", nil
	}
	name, exists := dicttls.DictCipherSuiteValueIndexed[id]
	if _, allowed := cipherVersions(name); !exists || !allowed {
		return "", fmt.Errorf("ClientHello contains an unsupported cipher suite")
	}
	return name, nil
}

func captureGroupName(id uint16) (string, error) {
	if captureGREASE(id) {
		return "GREASE", nil
	}
	for _, name := range []string{"X25519MLKEM768", "x25519", "secp256r1", "secp384r1", "secp521r1"} {
		if group, ok := groupID(name); ok && uint16(group) == id {
			return name, nil
		}
	}
	return "", fmt.Errorf("ClientHello contains an unsupported named group")
}

func captureSignatureName(id uint16) (string, error) {
	name, exists := dicttls.DictSignatureSchemeValueIndexed[id]
	if _, allowed := signatureVersions(name); !exists || !allowed {
		return "", fmt.Errorf("ClientHello contains an unsupported signature scheme")
	}
	return name, nil
}

func captureVersionName(id uint16) (string, error) {
	switch {
	case captureGREASE(id):
		return "GREASE", nil
	case id == utls.VersionTLS12:
		return "TLS 1.2", nil
	case id == utls.VersionTLS13:
		return "TLS 1.3", nil
	default:
		return "", fmt.Errorf("ClientHello advertises an unsupported TLS version")
	}
}

func captureKeyBytes(id uint16) int {
	switch utls.CurveID(id) {
	case utls.X25519MLKEM768:
		return 1216
	case utls.X25519:
		return 32
	case utls.CurveP256:
		return 65
	case utls.CurveP384:
		return 97
	case utls.CurveP521:
		return 133
	default:
		return -1
	}
}

func captureZeroBytes(data []byte) bool {
	for _, value := range data {
		if value != 0 {
			return false
		}
	}
	return true
}
