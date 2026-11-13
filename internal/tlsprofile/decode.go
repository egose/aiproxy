package tlsprofile

import (
	"encoding/json"

	utls "github.com/refraction-networking/utls"
)

func decodeValidatedRecipe(data []byte) (spec *utls.ClientHelloSpec, err error) {
	defer func() {
		if recover() != nil {
			spec = nil
			err = recipeError("spec", "cannot decode a supported ClientHello")
		}
	}()
	var envelope struct {
		CipherSuites utls.CipherSuitesJSONUnmarshaler       `json:"cipher_suites"`
		Compression  utls.CompressionMethodsJSONUnmarshaler `json:"compression_methods"`
		Extensions   []json.RawMessage                      `json:"extensions"`
		Min          uint16                                 `json:"min_vers"`
		Max          uint16                                 `json:"max_vers"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, recipeError("spec", "cannot decode supported cipher suites or compression")
	}
	spec = &utls.ClientHelloSpec{
		CipherSuites:       envelope.CipherSuites.CipherSuites(),
		CompressionMethods: envelope.Compression.CompressionMethods(),
		TLSVersMin:         envelope.Min,
		TLSVersMax:         envelope.Max,
	}
	for i, raw := range envelope.Extensions {
		field := recipeIndex("extensions", i)
		var entry struct {
			Name   string   `json:"name"`
			Groups []string `json:"named_group_list"`
			Shares []struct {
				Group string  `json:"group"`
				Data  []uint8 `json:"key_exchange"`
			} `json:"client_shares"`
		}
		if err := json.Unmarshal(raw, &entry); err != nil {
			return nil, recipeError(field, "cannot decode supported extension")
		}
		switch entry.Name {
		case "supported_groups":
			extension := &utls.SupportedCurvesExtension{Curves: make([]utls.CurveID, len(entry.Groups))}
			for index, name := range entry.Groups {
				id, ok := groupID(name)
				if !ok {
					return nil, recipeError(recipeIndex(field+".named_group_list", index), "unsupported named group")
				}
				extension.Curves[index] = id
			}
			spec.Extensions = append(spec.Extensions, extension)
		case "key_share":
			extension := &utls.KeyShareExtension{KeyShares: make([]utls.KeyShare, len(entry.Shares))}
			for index, share := range entry.Shares {
				id, ok := groupID(share.Group)
				if !ok {
					return nil, recipeError(recipeIndex(field+".client_shares", index)+".group", "unsupported key-share group")
				}
				extension.KeyShares[index] = utls.KeyShare{Group: id, Data: share.Data}
			}
			spec.Extensions = append(spec.Extensions, extension)
		default:
			var extension utls.TLSExtensionsJSONUnmarshaler
			array := make([]byte, 0, len(raw)+2)
			array = append(array, '[')
			array = append(array, raw...)
			array = append(array, ']')
			if err := json.Unmarshal(array, &extension); err != nil {
				return nil, recipeError(field, "cannot decode supported extension")
			}
			spec.Extensions = append(spec.Extensions, extension.Extensions()...)
		}
	}
	return spec, nil
}

func groupID(name string) (utls.CurveID, bool) {
	switch name {
	case "GREASE":
		return utls.GREASE_PLACEHOLDER, true
	case "X25519MLKEM768":
		return utls.X25519MLKEM768, true
	case "x25519":
		return utls.X25519, true
	case "secp256r1":
		return utls.CurveP256, true
	case "secp384r1":
		return utls.CurveP384, true
	case "secp521r1":
		return utls.CurveP521, true
	default:
		return 0, false
	}
}
