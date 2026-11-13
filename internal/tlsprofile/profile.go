package tlsprofile

import (
	"crypto/sha256"
	"fmt"

	utls "github.com/refraction-networking/utls"
)

const (
	DependencyVersion   = "v1.8.2"
	SchemaVersion       = "1"
	PolicyVersion       = "1"
	MaxSpecBytes        = 64 << 10
	MaxClientHelloBytes = 16 << 10
)

type Profile struct {
	name     string
	recipe   string
	identity string
}

func Native() Profile {
	return Profile{}
}

func BuiltinSelectors() []string {
	return []string{"native", "chrome", "chrome-133", "safari", "safari-16.0"}
}

func ResolveBuiltin(selection string) (Profile, error) {
	var name string
	switch selection {
	case "native":
		return Native(), nil
	case "chrome", "chrome-133":
		name = "chrome-133"
	case "safari", "safari-16.0":
		name = "safari-16.0"
	default:
		return Profile{}, fmt.Errorf("tls_client_hello: unsupported built-in selector")
	}
	return Profile{name: name, identity: profileIdentity(name, "")}, nil
}

func ParseCustom(data []byte) (Profile, error) {
	canonical, err := validateRecipe(data)
	if err != nil {
		return Profile{}, err
	}
	spec, err := decodeValidatedRecipe(canonical)
	if err != nil {
		return Profile{}, err
	}
	if err := validateClientHelloSize(spec); err != nil {
		return Profile{}, err
	}
	recipe := string(canonical)
	return Profile{name: "custom", recipe: recipe, identity: profileIdentity("custom", recipe)}, nil
}

func DecodeSpec(data []byte) (*utls.ClientHelloSpec, error) {
	profile, err := ParseCustom(data)
	if err != nil {
		return nil, err
	}
	return profile.Materialize()
}

func (p Profile) IsNative() bool {
	return p.name == ""
}

func (p Profile) IsCustom() bool {
	return p.name == "custom"
}

func (p Profile) Identity() string {
	if p.IsNative() {
		return profileIdentity("native", "")
	}
	return p.identity
}

func (p Profile) CanonicalName() string {
	if p.IsNative() {
		return "native"
	}
	return p.name
}

func (p Profile) CanonicalJSON() []byte {
	if !p.IsCustom() {
		return nil
	}
	return []byte(p.recipe)
}

func (p Profile) Materialize() (*utls.ClientHelloSpec, error) {
	var id utls.ClientHelloID
	switch p.name {
	case "":
		return nil, fmt.Errorf("tls_client_hello: native uses standard Go TLS, not a uTLS specification")
	case "chrome-133":
		id = utls.HelloChrome_133
	case "safari-16.0":
		id = utls.HelloSafari_16_0
	case "custom":
		return decodeValidatedRecipe([]byte(p.recipe))
	default:
		return nil, fmt.Errorf("tls_client_hello: invalid effective profile")
	}
	spec, err := utls.UTLSIdToSpec(id)
	if err != nil {
		return nil, fmt.Errorf("tls_client_hello: bundled profile is unavailable")
	}
	return &spec, nil
}

func profileIdentity(name, recipe string) string {
	digest := sha256.Sum256([]byte("aiproxy/tls-client-hello\x00utls=" + DependencyVersion + "\x00schema=" + SchemaVersion + "\x00policy=" + PolicyVersion + "\x00" + name + "\x00" + recipe))
	return name + ":" + fmt.Sprintf("%x", digest)
}
