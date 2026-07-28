package config

import (
	"fmt"
	"os"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

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

	file, diags := hclsyntax.ParseConfig(expanded, filename, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, fmt.Errorf("parse config %s: %s", filename, diags.Error())
	}

	var raw rawFile
	if d := gohcl.DecodeBody(file.Body, nil, &raw); d.HasErrors() {
		return nil, fmt.Errorf("decode config %s: %s", filename, d.Error())
	}

	rt, err := buildRuntime(&raw)
	if err != nil {
		return nil, err
	}
	if err := Validate(rt); err != nil {
		return nil, err
	}
	return rt, nil
}
