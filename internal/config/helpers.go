package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// expandEnvCalls replaces env("VAR") calls in the HCL source with the literal
// environment value, so secret injection works without a full HCL evaluation
// context. Missing variables expand to the empty string.
func expandEnvCalls(src []byte) []byte {
	re := regexp.MustCompile(`env\("([^"]+)"\)`)
	return re.ReplaceAllFunc(src, func(match []byte) []byte {
		sub := re.FindSubmatch(match)
		if len(sub) < 2 {
			return []byte(`""`)
		}
		val := os.Getenv(string(sub[1]))
		return []byte(strconv.Quote(val))
	})
}

func trimLeadingWhitespace(src []byte) []byte {
	for len(src) > 0 && (src[0] == '\n' || src[0] == '\r' || src[0] == ' ' || src[0] == '\t') {
		src = src[1:]
	}
	return src
}

// resolveProviderCredential materializes the effective API key for a provider.
// If both api_key and api_key_ref are set, the loader rejects the config.
// Validation enforces the "exactly one" rule; resolution just reads the file.
func resolveProviderCredential(p *Provider) error {
	if p.APIKey != "" && p.APIKeyRef != nil {
		return fmt.Errorf("only one of api_key or api_key_ref may be set")
	}
	if p.APIKeyRef == nil {
		return nil
	}
	if p.APIKeyRef.Key == "" {
		return fmt.Errorf("api_key_ref.key is required")
	}

	data, err := os.ReadFile(p.APIKeyRef.Path)
	if err != nil {
		return fmt.Errorf("read api_key_ref file %q: %w", p.APIKeyRef.Path, err)
	}

	v, err := extractKeyValue(data, p.APIKeyRef.Key)
	if err != nil {
		return fmt.Errorf("api_key_ref in %q: %w", p.APIKeyRef.Path, err)
	}
	if v == "" {
		return fmt.Errorf("api_key_ref key %q is empty in %q", p.APIKeyRef.Key, p.APIKeyRef.Path)
	}
	p.APIKey = v
	p.APIKeyRef.Resolved = true
	return nil
}

// defaultKeyFilePath returns the secure default location for the keys.json
// file, honoring XDG_CONFIG_HOME with fallback to ~/.config/aiproxy/keys.json.
func defaultKeyFilePath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "aiproxy", "keys.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "aiproxy/keys.json"
	}
	return filepath.Join(home, ".config", "aiproxy", "keys.json")
}

func parseTimeouts(t *rawTimeouts) (Timeouts, error) {
	out := Timeouts{}
	if t.ReadHeader != "" {
		d, err := time.ParseDuration(t.ReadHeader)
		if err != nil {
			return out, fmt.Errorf("invalid read_header timeout: %w", err)
		}
		out.ReadHeader = d
	}
	if t.Idle != "" {
		d, err := time.ParseDuration(t.Idle)
		if err != nil {
			return out, fmt.Errorf("invalid idle timeout: %w", err)
		}
		out.Idle = d
	}
	if t.Write != "" {
		d, err := time.ParseDuration(t.Write)
		if err != nil {
			return out, fmt.Errorf("invalid write timeout: %w", err)
		}
		out.Write = d
	}
	return out, nil
}

// nameRule is the shared rule for provider, alias, and model block labels.
var nameRule = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

func isLowercaseName(name string) bool {
	if strings.Contains(name, "/") || strings.ContainsAny(name, " \t\r\n") {
		return false
	}
	return nameRule.MatchString(name)
}
