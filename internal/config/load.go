package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/json"
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

	var raw rawFile
	if d := gohcl.DecodeBody(file.Body, nil, &raw); d.HasErrors() {
		return nil, fmt.Errorf("decode config %s: %s", filename, d.Error())
	}
	annotateProviderSyntax(file.Body, &raw)

	rt, err := buildRuntime(&raw)
	if err != nil {
		return nil, err
	}
	if err := Validate(rt); err != nil {
		return nil, err
	}
	return rt, nil
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
				{Name: "upstream_header_timeout"},
				{Name: "user_agent"},
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
