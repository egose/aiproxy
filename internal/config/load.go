package config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egose/aiproxy/internal/tlsprofile"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/json"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
)

const ConfigEnvVar = "AIPROXY_CONFIG"

const EnvConfigFilename = "env(AIPROXY_CONFIG)"

func EnvConfigContent() (string, bool) {
	src, ok := os.LookupEnv(ConfigEnvVar)
	if !ok || strings.TrimSpace(src) == "" {
		return "", false
	}
	return src, true
}

func LoadEnv() (*Runtime, error) {
	src, ok := EnvConfigContent()
	if !ok {
		return nil, fmt.Errorf("config %s is not set", EnvConfigFilename)
	}
	return Load([]byte(src), EnvConfigFilename)
}

func LoadFileOrEnv(path string, explicit bool) (*Runtime, error) {
	if !explicit {
		if _, ok := EnvConfigContent(); ok {
			return LoadEnv()
		}
	}
	return LoadFile(path)
}

func LoadFile(path string) (*Runtime, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	return Load(src, path)
}

func Load(src []byte, filename string) (*Runtime, error) {
	src = trimLeadingWhitespace(src)
	expanded := expandEnvCalls(src)

	file, err := parseConfig(expanded, filename)
	if err != nil {
		return nil, err
	}
	if err := validateProviderTLSAttributes(file.Body); err != nil {
		return nil, err
	}

	var raw rawFile
	if d := gohcl.DecodeBody(file.Body, nil, &raw); d.HasErrors() {
		return nil, fmt.Errorf("decode config %s: %s", filename, d.Error())
	}
	annotateProviderSyntax(file.Body, &raw)

	rt, err := buildRuntime(&raw, filename)
	if err != nil {
		return nil, err
	}
	if err := Validate(rt); err != nil {
		return nil, err
	}
	return rt, nil
}

func ValidateProviderTLSProfiles(src []byte, filename string) error {
	file, err := parseConfig(expandEnvCalls(trimLeadingWhitespace(src)), filename)
	if err != nil {
		return fmt.Errorf("tls_client_hello: cannot parse provider declarations")
	}
	if err := validateProviderTLSAttributes(file.Body); err != nil {
		return err
	}
	content, _, diags := file.Body.PartialContent(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{{Type: "provider", LabelNames: []string{"type", "name"}}},
	})
	if diags.HasErrors() {
		return fmt.Errorf("tls_client_hello: cannot inspect provider declarations")
	}
	for _, block := range content.Blocks {
		attrs, _, diags := block.Body.PartialContent(&hcl.BodySchema{Attributes: []hcl.AttributeSchema{
			{Name: "tls_client_hello"}, {Name: "tls_client_hello_spec_file"},
		}})
		if diags.HasErrors() {
			return fmt.Errorf("provider %q: tls_client_hello: cannot inspect declarations", block.Labels[1])
		}
		p := Provider{Name: block.Labels[1]}
		if attr := attrs.Attributes["tls_client_hello"]; attr != nil {
			value, err := providerTLSAttributeString(attr, p.Name, false)
			if err != nil {
				return err
			}
			p.TLSClientHello, p.HasTLSClientHello = value, true
		}
		if attr := attrs.Attributes["tls_client_hello_spec_file"]; attr != nil {
			value, err := providerTLSAttributeString(attr, p.Name, false)
			if err != nil {
				return err
			}
			p.TLSClientHelloSpecFile, p.HasTLSClientHelloSpecFile = value, true
		}
		if _, err := buildProviderTLSProfile(p, filename); err != nil {
			return fmt.Errorf("provider %q: %w", p.Name, err)
		}
	}
	return nil
}

func parseConfig(src []byte, filename string) (*hcl.File, error) {
	file, diags := hclsyntax.ParseConfig(src, filename, hcl.Pos{Line: 1, Column: 1})
	if !diags.HasErrors() {
		return file, nil
	}
	if jsonFile, jsonDiags := json.Parse(src, filename); !jsonDiags.HasErrors() {
		return jsonFile, nil
	}
	return nil, fmt.Errorf("parse config %s: %s", filename, diags.Error())
}

func annotateProviderSyntax(body hcl.Body, raw *rawFile) {
	raw.providerSyntax = make([]rawProviderSyntax, 0, len(raw.Providers))
	if syntaxBody, ok := body.(*hclsyntax.Body); ok {
		for _, block := range syntaxBody.Blocks {
			if block.Type != "provider" {
				continue
			}
			attrs := make(map[string]bool, len(block.Body.Attributes))
			for name := range block.Body.Attributes {
				attrs[name] = true
			}
			blocks := make(map[string]int)
			for _, nested := range block.Body.Blocks {
				blocks[nested.Type]++
			}
			raw.providerSyntax = append(raw.providerSyntax, rawProviderSyntax{Attrs: attrs, Blocks: blocks})
		}
		return
	}
	content, _, diags := body.PartialContent(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{{Type: "provider", LabelNames: []string{"type", "name"}}},
	})
	if diags.HasErrors() {
		return
	}
	for _, block := range content.Blocks {
		attrs := make(map[string]bool)
		if attrContent, _, attrDiags := block.Body.PartialContent(&hcl.BodySchema{
			Attributes: []hcl.AttributeSchema{
				{Name: "extends"},
				{Name: "display_name"},
				{Name: "base_url"},
				{Name: "allow_insecure_base_url"},
				{Name: "tls_client_hello"},
				{Name: "tls_client_hello_spec_file"},
				{Name: "upstream_header_timeout"},
				{Name: "user_agent"},
				{Name: "forward_headers"},
				{Name: "api_key"},
				{Name: "enabled"},
			},
		}); !attrDiags.HasErrors() {
			for name := range attrContent.Attributes {
				attrs[name] = true
			}
		}
		blocks := make(map[string]int)
		if blockContent, _, blockDiags := block.Body.PartialContent(&hcl.BodySchema{
			Blocks: []hcl.BlockHeaderSchema{
				{Type: "api_key_ref"},
				{Type: "credential_ref"},
				{Type: "healthcheck"},
				{Type: "model", LabelNames: []string{"name"}},
			},
		}); !blockDiags.HasErrors() {
			for _, nested := range blockContent.Blocks {
				blocks[nested.Type]++
			}
		}
		raw.providerSyntax = append(raw.providerSyntax, rawProviderSyntax{Attrs: attrs, Blocks: blocks})
	}
}

func validateProviderTLSAttributes(body hcl.Body) error {
	root, _, rootDiags := body.PartialContent(&hcl.BodySchema{Attributes: []hcl.AttributeSchema{
		{Name: "tls_client_hello"}, {Name: "tls_client_hello_spec_file"},
	}})
	if rootDiags.HasErrors() {
		return fmt.Errorf("tls_client_hello: cannot inspect root declarations")
	}
	for _, name := range []string{"tls_client_hello", "tls_client_hello_spec_file"} {
		if root.Attributes[name] != nil {
			return fmt.Errorf("%s is provider-only", name)
		}
	}
	content, _, diags := body.PartialContent(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{{Type: "provider", LabelNames: []string{"type", "name"}}},
	})
	if diags.HasErrors() {
		return fmt.Errorf("tls_client_hello: cannot inspect provider declarations")
	}
	for _, block := range content.Blocks {
		attrs, _, diags := block.Body.PartialContent(&hcl.BodySchema{Attributes: []hcl.AttributeSchema{
			{Name: "extends"}, {Name: "tls_client_hello"}, {Name: "tls_client_hello_spec_file"},
		}})
		if diags.HasErrors() {
			return fmt.Errorf("provider %q: tls_client_hello: cannot inspect declarations", block.Labels[1])
		}
		derived := false
		if extends := attrs.Attributes["extends"]; extends != nil {
			value, err := providerTLSAttributeString(extends, block.Labels[1], true)
			if err != nil {
				return err
			}
			derived = value != ""
		}
		for _, name := range []string{"tls_client_hello", "tls_client_hello_spec_file"} {
			attr := attrs.Attributes[name]
			if attr == nil {
				continue
			}
			if derived {
				return fmt.Errorf("provider %q: derived provider cannot declare %s", block.Labels[1], name)
			}
			if _, err := providerTLSAttributeString(attr, block.Labels[1], false); err != nil {
				return err
			}
		}
	}
	return nil
}

func providerTLSAttributeString(attr *hcl.Attribute, providerName string, coerce bool) (string, error) {
	invalid := fmt.Errorf("provider %q: %s must be a non-null string", providerName, attr.Name)
	value, diags := attr.Expr.Value(nil)
	if diags.HasErrors() || !value.IsKnown() || value.IsNull() {
		return "", invalid
	}
	if coerce {
		var err error
		value, err = convert.Convert(value, cty.String)
		if err != nil {
			return "", invalid
		}
	}
	if !value.IsKnown() || value.IsNull() || value.Type() != cty.String {
		return "", invalid
	}
	return value.AsString(), nil
}

func validateTLSProfileReference(reference string) error {
	if reference == "" || len(reference) > 4096 || !utf8.ValidString(reference) {
		return fmt.Errorf("tls_client_hello_spec_file must be a nonempty UTF-8 reference of at most 4096 bytes")
	}
	for _, char := range reference {
		if unicode.IsControl(char) {
			return fmt.Errorf("tls_client_hello_spec_file must not contain control characters")
		}
	}
	return nil
}

func resolveTLSProfileReference(reference, filename string) (string, error) {
	if err := validateTLSProfileReference(reference); err != nil {
		return "", err
	}
	if filepath.IsAbs(reference) {
		return filepath.Clean(reference), nil
	}
	if filename == "" || filename == EnvConfigFilename {
		return "", fmt.Errorf("tls_client_hello_spec_file requires an absolute reference for inline or source-less config")
	}
	source, err := filepath.Abs(filename)
	if err != nil {
		return "", fmt.Errorf("tls_client_hello_spec_file cannot resolve the config source directory")
	}
	return filepath.Join(filepath.Dir(source), reference), nil
}

func loadTLSProfile(reference, filename string) (tlsprofile.Profile, error) {
	path, err := resolveTLSProfileReference(reference, filename)
	if err != nil {
		return tlsprofile.Profile{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return tlsprofile.Profile{}, fmt.Errorf("tls_client_hello_spec_file cannot inspect the source file")
	}
	if !info.Mode().IsRegular() {
		return tlsprofile.Profile{}, fmt.Errorf("tls_client_hello_spec_file must refer to a regular file")
	}
	file, err := openTLSProfileFile(path)
	if err != nil {
		return tlsprofile.Profile{}, fmt.Errorf("tls_client_hello_spec_file cannot open the source file")
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil {
		return tlsprofile.Profile{}, fmt.Errorf("tls_client_hello_spec_file cannot inspect the opened file")
	}
	if !info.Mode().IsRegular() {
		return tlsprofile.Profile{}, fmt.Errorf("tls_client_hello_spec_file must refer to a regular file")
	}
	if info.Size() > tlsprofile.MaxSpecBytes {
		return tlsprofile.Profile{}, fmt.Errorf("tls_client_hello_spec_file must be at most 65536 bytes")
	}
	data, err := io.ReadAll(io.LimitReader(file, tlsprofile.MaxSpecBytes+1))
	if err != nil {
		return tlsprofile.Profile{}, fmt.Errorf("tls_client_hello_spec_file cannot read the source file")
	}
	profile, err := tlsprofile.ParseCustom(data)
	if err != nil {
		return tlsprofile.Profile{}, fmt.Errorf("tls_client_hello_spec_file: %w", err)
	}
	return profile, nil
}

func buildProviderTLSProfile(p Provider, filename string) (tlsprofile.Profile, error) {
	selection, err := providerTLSSelection(p)
	if err != nil {
		return tlsprofile.Profile{}, err
	}
	if selection == "custom" {
		return loadTLSProfile(p.TLSClientHelloSpecFile, filename)
	}
	return tlsprofile.ResolveBuiltin(selection)
}
