package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/egose/aiproxy/internal/copilotlogin"
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

// validateProviderCredentialStructure enforces the "exactly one credential"
// rule and required api_key_ref.key without reading any file. Disabled
// providers must still pass these structural checks.
func validateProviderCredentialStructure(p *Provider) error {
	if p.Type == ProviderTypeGitHubCopilot {
		if p.APIKey != "" || p.APIKeyRef != nil { // pragma: allowlist secret
			return fmt.Errorf("api_key and api_key_ref are not supported by github-copilot; use credential_ref")
		}
		if p.CopilotCredentialRef == nil {
			return nil
		}
		if p.CopilotCredentialRef.Name == "" {
			return fmt.Errorf("credential_ref.name is required")
		}
		return nil
	}
	if p.CopilotCredentialRef != nil {
		return fmt.Errorf("credential_ref is only supported by github-copilot")
	}
	if p.APIKey != "" && p.APIKeyRef != nil { // pragma: allowlist secret
		return fmt.Errorf("only one of api_key or api_key_ref may be set")
	}
	if p.APIKeyRef == nil { // pragma: allowlist secret
		return nil
	}
	if p.APIKeyRef.Key == "" {
		return fmt.Errorf("api_key_ref.key is required")
	}
	return nil
}

// resolveProviderCredential materializes the effective API key for an enabled
// provider. Structural checks must already have passed via
// validateProviderCredentialStructure.
func resolveProviderCredential(p *Provider) error {
	if p.APIKeyRef == nil { // pragma: allowlist secret
		return nil
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

// resolveCopilotCredential materializes the effective OAuth token for an
// enabled github-copilot provider from its sidecar credential file. The
// token is stored on the dedicated CopilotToken field, never on APIKey.
func resolveCopilotCredential(p *Provider) error {
	if p.CopilotCredentialRef == nil {
		return nil
	}
	secretsPath := p.CopilotCredentialRef.Path // pragma: allowlist secret
	if secretsPath == "" {
		secretsPath = defaultKeyFilePath()
		p.CopilotCredentialRef.Path = secretsPath
	}
	cred, err := copilotlogin.Load(secretsPath, p.CopilotCredentialRef.Name)
	if err != nil {
		return fmt.Errorf("credential_ref %q: %w (run login first, then restart or SIGHUP)", p.CopilotCredentialRef.Name, err)
	}
	if cred.ExpiresAt != 0 && cred.ExpiresAt <= time.Now().Unix() {
		return fmt.Errorf("credential_ref %q: credential is expired; re-run login, then restart or SIGHUP", p.CopilotCredentialRef.Name)
	}
	p.CopilotToken = cred.AccessToken
	p.CopilotCredentialRef.Resolved = true
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
		if d < 0 {
			return out, fmt.Errorf("invalid read_header timeout: must not be negative")
		}
		out.ReadHeader = d
	}
	if t.Idle != "" {
		d, err := time.ParseDuration(t.Idle)
		if err != nil {
			return out, fmt.Errorf("invalid idle timeout: %w", err)
		}
		if d < 0 {
			return out, fmt.Errorf("invalid idle timeout: must not be negative")
		}
		out.Idle = d
	}
	if t.Write != "" {
		d, err := time.ParseDuration(t.Write)
		if err != nil {
			return out, fmt.Errorf("invalid write timeout: %w", err)
		}
		if d < 0 {
			return out, fmt.Errorf("invalid write timeout: must not be negative")
		}
		out.Write = d
	}
	return out, nil
}

func parsePositiveDuration(field, value string) (time.Duration, error) {
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", field, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("invalid %s: must be greater than zero", field)
	}
	return d, nil
}

// nameRule is the shared rule for provider, alias, and model block labels.
var nameRule = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

func IsLowercaseName(name string) bool {
	if strings.Contains(name, "/") || strings.ContainsAny(name, " \t\r\n") {
		return false
	}
	return nameRule.MatchString(name)
}

func IsLowercaseModelName(name string) bool {
	if strings.ContainsAny(name, " \t\r\n") {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || !nameRule.MatchString(part) {
			return false
		}
	}
	return true
}
