package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/egose/aiproxy/internal/copilotlogin"
)

var ErrCopilotCredential = errors.New("invalid encrypted Copilot credential; authorize again and verify database encryption key")

type copilotCredentialEnvelope struct {
	Version    int                     `json:"version"`
	Kind       string                  `json:"kind"`
	Credential copilotlogin.Credential `json:"credential"`
}

func EncryptCopilotCredential(cred copilotlogin.Credential, now time.Time) ([]byte, error) {
	if err := copilotlogin.ValidateDatabaseCredential(cred, now); err != nil {
		return nil, ErrCopilotCredential
	}
	plain, err := json.Marshal(copilotCredentialEnvelope{Version: 1, Kind: "github-copilot", Credential: cred})
	if err != nil {
		return nil, ErrCopilotCredential
	}
	encrypted, err := EncryptSecret(plain)
	if err != nil {
		return nil, ErrCopilotCredential
	}
	return encrypted, nil
}

func DecryptCopilotCredential(encrypted []byte, now time.Time) (copilotlogin.Credential, error) {
	plain, err := DecryptSecret(encrypted)
	if err != nil {
		return copilotlogin.Credential{}, ErrCopilotCredential
	}
	var envelope copilotCredentialEnvelope
	decoder := json.NewDecoder(bytes.NewReader(plain))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return copilotlogin.Credential{}, ErrCopilotCredential
	}
	if decoder.Decode(new(any)) != io.EOF || envelope.Version != 1 || envelope.Kind != "github-copilot" ||
		copilotlogin.ValidateDatabaseCredential(envelope.Credential, now) != nil {
		return copilotlogin.Credential{}, ErrCopilotCredential
	}
	return envelope.Credential, nil
}
