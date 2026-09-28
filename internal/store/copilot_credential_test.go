package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/copilotlogin"
)

func TestCopilotCredentialCrypto(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	t.Setenv("AIPROXY_DB_ENCRYPTION_KEY", "copilot-fixture-key")
	cred, err := copilotlogin.NewCredential("client-id", "gho_private_fixture", now)
	if err != nil {
		t.Fatal(err)
	}
	cred.RefreshToken = "refresh_private_fixture"
	cred.ExpiresAt = now.Add(time.Hour).Unix()
	enc, err := EncryptCopilotCredential(cred, now)
	if err != nil {
		t.Fatal(err)
	}
	again, err := EncryptCopilotCredential(cred, now)
	if err != nil || bytes.Equal(enc, again) || bytes.Contains(enc, []byte(cred.AccessToken)) || bytes.Contains(enc, []byte(cred.RefreshToken)) {
		t.Fatal("encryption must use fresh nonces and hide tokens")
	}
	got, err := DecryptCopilotCredential(enc, now)
	if err != nil || got != cred {
		t.Fatal("structured roundtrip failed")
	}
	for name, mutate := range map[string]func(*copilotlogin.Credential){
		"domain":           func(c *copilotlogin.Credential) { c.Domain = "other.example" },
		"missing client":   func(c *copilotlogin.Credential) { c.ClientID = "" },
		"bad client":       func(c *copilotlogin.Credential) { c.ClientID = "a b" },
		"missing access":   func(c *copilotlogin.Credential) { c.AccessToken = "" },
		"missing refresh":  func(c *copilotlogin.Credential) { c.RefreshToken = "" },
		"header injection": func(c *copilotlogin.Credential) { c.AccessToken = "secret\r\nheader" },
		"bad refresh":      func(c *copilotlogin.Credential) { c.RefreshToken = "secret\x00" },
		"negative expiry":  func(c *copilotlogin.Credential) { c.ExpiresAt = -1 },
		"expired":          func(c *copilotlogin.Credential) { c.ExpiresAt = now.Unix() },
		"missing obtained": func(c *copilotlogin.Credential) { c.ObtainedAt = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			bad := cred
			mutate(&bad)
			if result, err := EncryptCopilotCredential(bad, now); err != ErrCopilotCredential || result != nil {
				t.Fatal("invalid credential accepted for encryption or unsafe error")
			}
			plain, err := json.Marshal(copilotCredentialEnvelope{Version: 1, Kind: "github-copilot", Credential: bad})
			if err != nil {
				t.Fatal(err)
			}
			blob, err := EncryptSecret(plain)
			if err != nil {
				t.Fatal(err)
			}
			assertInvalidCopilotBlob(t, blob, now)
		})
	}
	for _, plain := range []string{"not-json-private", `null`, `{}`, `{"version":2,"kind":"github-copilot"}`, `{"version":1,"kind":"device-challenge"}`} {
		blob, err := EncryptSecret([]byte(plain))
		if err != nil {
			t.Fatal(err)
		}
		assertInvalidCopilotBlob(t, blob, now)
	}
	plain, err := DecryptSecret(enc)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]byte{append(append([]byte(nil), plain...), []byte(` {}`)...), bytes.Replace(plain, []byte(`"version":1`), []byte(`"version":9`), 1), bytes.Replace(plain, []byte(`"kind":"github-copilot"`), []byte(`"kind":"device-challenge"`), 1), bytes.Replace(plain, []byte(`"version":1`), []byte(`"version":1,"unknown":"secret"`), 1)} {
		blob, err := EncryptSecret(bad)
		if err != nil {
			t.Fatal(err)
		}
		assertInvalidCopilotBlob(t, blob, now)
	}
	assertInvalidCopilotBlob(t, nil, now)
	assertInvalidCopilotBlob(t, []byte("short-private"), now)
	assertInvalidCopilotBlob(t, enc[:len(enc)-1], now)
	tampered := append([]byte(nil), enc...)
	tampered[len(tampered)-1] ^= 1
	assertInvalidCopilotBlob(t, tampered, now)
	assertInvalidCopilotBlob(t, enc, now.Add(time.Hour))
	t.Setenv("AIPROXY_DB_ENCRYPTION_KEY", "wrong-key")
	assertInvalidCopilotBlob(t, enc, now)
	t.Setenv("AIPROXY_DB_ENCRYPTION_KEY", "")
	t.Setenv("AIPROXY_JWT_SECRET", "")
	assertInvalidCopilotBlob(t, enc, now)
	if _, err := EncryptCopilotCredential(cred, now); err != ErrCopilotCredential {
		t.Fatal("missing key accepted")
	}
	t.Setenv("AIPROXY_JWT_SECRET", "copilot-fixture-key")
	if got, err := DecryptCopilotCredential(enc, now); err != nil || got != cred {
		t.Fatal("existing key fallback changed")
	}
}

func assertInvalidCopilotBlob(t *testing.T, blob []byte, now time.Time) {
	t.Helper()
	got, err := DecryptCopilotCredential(blob, now)
	if !errors.Is(err, ErrCopilotCredential) || err.Error() != ErrCopilotCredential.Error() || got != (copilotlogin.Credential{}) {
		t.Fatal("invalid blob must return a controlled error and zero credential")
	}
}

func TestCopilotCredentialSchemaAndPersistence(t *testing.T) {
	st := openLedgerTestStore(t)
	ctx := context.Background()
	if applied, err := st.MigrateUp(ctx); err != nil || len(applied) != 0 {
		t.Fatal("schema is not idempotent")
	}
	status, err := st.Status(ctx)
	if err != nil || !reflect.DeepEqual(status.Applied, []string{"schema.sql"}) || len(status.Pending) != 0 {
		t.Fatalf("schema status = %+v, %v", status, err)
	}
	for _, row := range []struct {
		name, typ, ref string
		enabled        bool
	}{
		{"existing-file", "github-copilot", "local", true},
		{"existing-disabled", "github-copilot", "", false},
		{"existing-api", "openai", "", true},
	} {
		if _, err := st.DB.ExecContext(ctx, `INSERT INTO db_providers (name,type,copilot_credential_name,enabled) VALUES (?,?,?,?)`, row.name, row.typ, row.ref, row.enabled); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := st.ListProviders(ctx)
	if err != nil || len(rows) != 3 {
		t.Fatal("existing rows lost")
	}
	for _, row := range rows {
		if row.CopilotCredentialEncrypted != nil {
			t.Fatal("existing row gained encrypted source")
		}
		if row.Name == "existing-file" && row.CopilotCredentialName != "local" {
			t.Fatal("sidecar changed")
		}
	}
	t.Setenv("AIPROXY_DB_ENCRYPTION_KEY", "copilot-fixture-key")
	now := time.Now()
	cred, err := copilotlogin.NewCredential("client-id", "private-db-token", now)
	if err != nil {
		t.Fatal(err)
	}
	enc, err := EncryptCopilotCredential(cred, now)
	if err != nil {
		t.Fatal(err)
	}
	row := DBProvider{Name: "database", Type: "github-copilot", Enabled: true, CopilotCredentialEncrypted: enc}
	if err := st.CreateProviderAggregate(ctx, &row, nil); err != nil {
		t.Fatal(err)
	}
	stored, err := st.GetProvider(ctx, row.Name)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored.CopilotCredentialEncrypted, enc) || len(stored.APIKeyEncrypted) != 0 {
		t.Fatal("wrong storage column")
	}
	got, err := DecryptCopilotCredential(stored.CopilotCredentialEncrypted, now)
	if err != nil || got != cred {
		t.Fatal("database credential roundtrip failed")
	}
	encoded, err := json.Marshal(stored)
	if err != nil || bytes.Contains(encoded, []byte("CopilotCredentialEncrypted")) || bytes.Contains(encoded, []byte(cred.AccessToken)) {
		t.Fatal("credential serialized")
	}
	stored.DisplayName = "edited"
	if err := st.UpdateProviderAggregate(ctx, &stored, nil); err != nil {
		t.Fatal(err)
	}
	updated, err := st.GetProvider(ctx, row.Name)
	if err != nil || !bytes.Equal(updated.CopilotCredentialEncrypted, enc) {
		t.Fatal("unrelated edit changed credential")
	}
	for name, mutate := range map[string]func(*DBProvider){
		"wrong type":   func(p *DBProvider) { p.Type = "openai" },
		"sidecar name": func(p *DBProvider) { p.CopilotCredentialName = "local" },
		"sidecar path": func(p *DBProvider) { p.CopilotCredentialPath = "/private" },
		"api blob":     func(p *DBProvider) { p.APIKeyEncrypted = []byte("private") },
		"api ref key":  func(p *DBProvider) { p.APIKeyRefKey = "local" },
		"api ref path": func(p *DBProvider) { p.APIKeyRefPath = "/private" },
		"empty blob":   func(p *DBProvider) { p.CopilotCredentialEncrypted = []byte{} },
	} {
		t.Run(name, func(t *testing.T) {
			bad := updated
			mutate(&bad)
			if err := st.UpdateProviderAggregate(ctx, &bad, nil); err == nil {
				t.Fatal("database accepted conflicting credential sources")
			}
			current, err := st.GetProvider(ctx, row.Name)
			if err != nil || current.UpdatedAt != updated.UpdatedAt || !bytes.Equal(current.CopilotCredentialEncrypted, enc) {
				t.Fatal("failed update changed row")
			}
		})
	}
}
