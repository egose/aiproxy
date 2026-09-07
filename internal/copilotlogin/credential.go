package copilotlogin

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const CredentialDomain = "github.com"

type Credential struct {
	ClientID     string `json:"client_id"`
	Domain       string `json:"domain"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
	ObtainedAt   int64  `json:"obtained_at"`
}

var credentialNameRule = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

func ValidateCredentialName(name string) error {
	if name == "" {
		return errors.New("credential name is required")
	}
	if strings.Contains(name, "/") || strings.ContainsAny(name, " \t\r\n") {
		return fmt.Errorf("invalid credential name %q: must be lowercase without spaces or slashes", name)
	}
	if !credentialNameRule.MatchString(name) {
		return fmt.Errorf("invalid credential name %q: must start with [a-z0-9] and contain only [a-z0-9._-]", name)
	}
	if len(name) > 128 {
		return fmt.Errorf("invalid credential name %q: too long", name)
	}
	return nil
}

func NewCredential(clientID, accessToken string, now time.Time) (Credential, error) {
	if err := validateClientID(clientID); err != nil {
		return Credential{}, err
	}
	if strings.TrimSpace(accessToken) == "" {
		return Credential{}, errors.New("access token is required")
	}
	if now.IsZero() {
		now = time.Now()
	}
	return Credential{
		ClientID:     clientID,
		Domain:       CredentialDomain,
		AccessToken:  accessToken,
		RefreshToken: accessToken,
		ExpiresAt:    0,
		ObtainedAt:   now.Unix(),
	}, nil
}

func SidecarPath(secretsPath, name string) (string, error) {
	if strings.TrimSpace(secretsPath) == "" {
		return "", errors.New("secrets path is required")
	}
	if err := ValidateCredentialName(name); err != nil {
		return "", err
	}
	dir := filepath.Dir(secretsPath)
	if dir == "" {
		return "", errors.New("invalid secrets path")
	}
	return filepath.Join(dir, "copilot-"+name+".json"), nil
}

func Save(secretsPath, name string, cred Credential) error {
	if err := ValidateCredentialName(name); err != nil {
		return err
	}
	if strings.TrimSpace(secretsPath) == "" {
		return errors.New("secrets path is required")
	}
	if err := validateStoredCredential(cred); err != nil {
		return err
	}
	dest, err := SidecarPath(secretsPath, name)
	if err != nil {
		return err
	}
	body, err := json.MarshalIndent(cred, "", "  ")
	if err != nil {
		return errors.New("encode credential")
	}
	body = append(body, '\n')
	if err := writeSecretFile(dest, body); err != nil {
		return err
	}
	return nil
}

func Load(secretsPath, name string) (Credential, error) {
	dest, err := SidecarPath(secretsPath, name)
	if err != nil {
		return Credential{}, err
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Credential{}, fmt.Errorf("credential %q not found; run login first", name)
		}
		return Credential{}, fmt.Errorf("read credential %q", name)
	}
	var cred Credential
	if err := json.Unmarshal(data, &cred); err != nil {
		return Credential{}, fmt.Errorf("parse credential %q", name)
	}
	if err := validateStoredCredential(cred); err != nil {
		return Credential{}, fmt.Errorf("invalid credential %q", name)
	}
	return cred, nil
}

func validateStoredCredential(cred Credential) error {
	if strings.TrimSpace(cred.ClientID) == "" {
		return errors.New("credential client_id is required")
	}
	if strings.TrimSpace(cred.Domain) == "" {
		return errors.New("credential domain is required")
	}
	if strings.TrimSpace(cred.AccessToken) == "" {
		return errors.New("credential access token is required")
	}
	if strings.TrimSpace(cred.RefreshToken) == "" {
		return errors.New("credential refresh token is required")
	}
	if cred.ExpiresAt < 0 || cred.ObtainedAt < 0 {
		return errors.New("credential timestamps are invalid")
	}
	return nil
}
