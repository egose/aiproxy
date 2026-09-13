package config

import (
	"encoding/json"
	"fmt"
	"math/big"
	"sort"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	hcljson "github.com/hashicorp/hcl/v2/json"
	"github.com/zclconf/go-cty/cty"
)

const (
	FormatHCL  = "hcl"
	FormatJSON = "json"
)

var jsonBlockLabels = map[string]int{
	"listener":         2,
	"auth":             1,
	"logging":          0,
	"provider_health":  0,
	"metrics":          0,
	"dashboard":        0,
	"provider":         2,
	"alias":            1,
	"timeouts":         0,
	"client":           1,
	"rate_limit":       0,
	"target":           0,
	"session_affinity": 0,
	"api_key_ref":      0,
	"credential_ref":   0,
	"healthcheck":      0,
	"model":            1,
	"payload_log":      0,
}

func Convert(src []byte, filename string, compact bool) ([]byte, string, string, error) {
	src = trimLeadingWhitespace(src)
	expanded := expandEnvCalls(src)
	if file, diags := hclsyntax.ParseConfig(expanded, filename, hcl.Pos{Line: 1, Column: 1}); !diags.HasErrors() {
		syntaxBody, ok := file.Body.(*hclsyntax.Body)
		if !ok {
			return nil, "", "", fmt.Errorf("parse config %s: unsupported body", filename)
		}
		obj, err := jsonBodyObject(syntaxBody)
		if err != nil {
			return nil, "", "", err
		}
		var out []byte
		if compact {
			out, err = json.Marshal(obj)
		} else {
			out, err = json.MarshalIndent(obj, "", "  ")
		}
		if err != nil {
			return nil, "", "", err
		}
		out = append(out, '\n')
		if err := checkConverted(out, filename, FormatJSON); err != nil {
			return nil, "", "", err
		}
		return out, FormatHCL, FormatJSON, nil
	}
	if _, jsonDiags := hcljson.Parse(expanded, filename); !jsonDiags.HasErrors() {
		out, err := decodeJSONConfig(expanded)
		if err != nil {
			return nil, "", "", err
		}
		if err := checkConverted(out, filename, FormatHCL); err != nil {
			return nil, "", "", err
		}
		return out, FormatJSON, FormatHCL, nil
	}
	_, err := parseConfig(expanded, filename)
	return nil, "", "", err
}

func checkConverted(out []byte, filename, format string) error {
	if _, err := Load(out, filename+".converted."+format); err != nil {
		return fmt.Errorf("converted config is invalid: %w", err)
	}
	return nil
}

func jsonBodyObject(body *hclsyntax.Body) (map[string]interface{}, error) {
	obj := make(map[string]interface{})
	for name, attr := range body.Attributes {
		val, err := jsonValue(attr.Expr)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if _, exists := obj[name]; exists {
			return nil, fmt.Errorf("%s: attribute collides with block", name)
		}
		obj[name] = val
	}
	for _, block := range body.Blocks {
		sub, err := jsonBodyObject(block.Body)
		if err != nil {
			return nil, err
		}
		path := make([]string, 0, len(block.Labels)+1)
		path = append(path, block.Type)
		path = append(path, block.Labels...)
		setJSONPath(obj, path, sub)
	}
	return obj, nil
}

func setJSONPath(obj map[string]interface{}, path []string, leaf interface{}) {
	cur := obj
	for _, key := range path[:len(path)-1] {
		next, ok := cur[key].(map[string]interface{})
		if !ok {
			next = make(map[string]interface{})
			cur[key] = next
		}
		cur = next
	}
	last := path[len(path)-1]
	if existing, ok := cur[last]; ok {
		if arr, ok := existing.([]interface{}); ok {
			cur[last] = append(arr, leaf)
		} else {
			cur[last] = []interface{}{existing, leaf}
		}
		return
	}
	cur[last] = leaf
}

func jsonValue(expr hcl.Expression) (interface{}, error) {
	val, diags := expr.Value(nil)
	if diags.HasErrors() {
		return nil, fmt.Errorf("cannot convert non-literal expression: %s", diags.Error())
	}
	return ctyToJSON(val)
}

func ctyToJSON(val cty.Value) (interface{}, error) {
	if val.IsNull() {
		return nil, nil
	}
	switch {
	case val.Type() == cty.String:
		return val.AsString(), nil
	case val.Type() == cty.Bool:
		return val.True(), nil
	case val.Type() == cty.Number:
		bf := val.AsBigFloat()
		if i, acc := bf.Int64(); acc == big.Exact {
			return i, nil
		}
		f, _ := bf.Float64()
		return f, nil
	case val.Type().IsTupleType() || val.Type().IsListType() || val.Type().IsSetType():
		out := make([]interface{}, 0)
		for it := val.ElementIterator(); it.Next(); {
			_, ev := it.Element()
			jv, err := ctyToJSON(ev)
			if err != nil {
				return nil, err
			}
			out = append(out, jv)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported value type %s", val.Type().FriendlyName())
	}
}

func decodeJSONConfig(src []byte) ([]byte, error) {
	var root interface{}
	if err := json.Unmarshal(src, &root); err != nil {
		return nil, err
	}
	obj, ok := root.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("JSON config must be an object")
	}
	f := hclwrite.NewEmptyFile()
	if err := writeHCLBody(f.Body(), obj, 0); err != nil {
		return nil, err
	}
	out := f.Bytes()
	if len(out) == 0 || out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	return out, nil
}

func writeHCLBody(body *hclwrite.Body, obj map[string]interface{}, depth int) error {
	for _, name := range sortedJSONKeys(obj) {
		val := obj[name]
		if isBlockValue(val) {
			arity, ok := jsonBlockLabels[name]
			if !ok {
				return fmt.Errorf("%s: unknown block", name)
			}
			if err := writeHCLBlocks(body, name, nil, val, arity, depth); err != nil {
				return err
			}
			continue
		}
		if arr, ok := val.([]interface{}); ok && len(arr) == 0 {
			if _, isBlock := jsonBlockLabels[name]; isBlock {
				continue
			}
		}
		cv, err := ctyForJSONValue(name, val)
		if err != nil {
			return err
		}
		body.SetAttributeValue(name, cv)
	}
	return nil
}

func isBlockValue(val interface{}) bool {
	switch tv := val.(type) {
	case map[string]interface{}:
		return true
	case []interface{}:
		for _, item := range tv {
			if _, ok := item.(map[string]interface{}); ok {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func writeHCLBlocks(body *hclwrite.Body, typeName string, labels []string, val interface{}, arity, depth int) error {
	if arity == 0 {
		switch tv := val.(type) {
		case map[string]interface{}:
			if err := writeHCLBlock(body, typeName, labels, tv, depth); err != nil {
				return err
			}
		case []interface{}:
			for _, item := range tv {
				sub, ok := item.(map[string]interface{})
				if !ok {
					return fmt.Errorf("%s block must be an object", typeName)
				}
				if err := writeHCLBlock(body, typeName, labels, sub, depth); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("%s block must be an object", typeName)
		}
		if depth == 0 {
			body.AppendNewline()
		}
		return nil
	}
	sub, ok := val.(map[string]interface{})
	if !ok {
		return fmt.Errorf("%s block expects %d labels", typeName, arity)
	}
	for _, label := range sortedJSONKeys(sub) {
		next := make([]string, len(labels)+1)
		copy(next, labels)
		next[len(labels)] = label
		if err := writeHCLBlocks(body, typeName, next, sub[label], arity-1, depth); err != nil {
			return err
		}
	}
	return nil
}

func writeHCLBlock(body *hclwrite.Body, typeName string, labels []string, obj map[string]interface{}, depth int) error {
	block := body.AppendNewBlock(typeName, labels)
	return writeHCLBody(block.Body(), obj, depth+1)
}

func sortedJSONKeys(obj map[string]interface{}) []string {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func ctyForJSONValue(name string, val interface{}) (cty.Value, error) {
	switch tv := val.(type) {
	case string:
		return cty.StringVal(tv), nil
	case bool:
		return cty.BoolVal(tv), nil
	case float64:
		if float64(int64(tv)) == tv {
			return cty.NumberIntVal(int64(tv)), nil
		}
		return cty.NumberFloatVal(tv), nil
	case []interface{}:
		elems := make([]cty.Value, 0, len(tv))
		for _, item := range tv {
			ev, err := ctyForJSONValue(name, item)
			if err != nil {
				return cty.DynamicVal, err
			}
			elems = append(elems, ev)
		}
		if len(elems) == 0 {
			return cty.ListValEmpty(cty.String), nil
		}
		return cty.TupleVal(elems), nil
	case nil:
		return cty.NullVal(cty.DynamicPseudoType), nil
	default:
		return cty.DynamicVal, fmt.Errorf("%s: cannot convert object to attribute value", name)
	}
}
