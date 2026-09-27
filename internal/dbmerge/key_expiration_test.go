package dbmerge

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

func TestMergedKeyExpirationAndLifecycle(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, testDBURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.MigrateUp(ctx); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Microsecond)
	past := time.Unix(1, 0)
	suffix := uuid.NewString()
	rows := []*store.InboundKey{
		{Name: "expiring-" + suffix, Enabled: true, ExpiresAt: &deadline},
		{Name: "permanent-" + suffix, Enabled: true},
		{Name: "expired-" + suffix, Enabled: true, ExpiresAt: &past},
		{Name: "disabled-" + suffix, Enabled: false, ExpiresAt: &deadline},
	}
	for _, row := range rows {
		row.TokenHash = store.TokenHash(row.Name)
		row.TokenPrefix = "fixture"
		if err := st.CreateInboundKey(ctx, row); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := st.DeleteInboundKey(ctx, row.ID); err != nil {
				t.Error(err)
			}
		})
	}
	now := deadline.Add(-time.Nanosecond)
	load := func() auth.Authenticator {
		t.Helper()
		merged, err := MergeCatalog(ctx, st, &config.Runtime{Catalog: config.NewCatalog(nil, nil, nil)})
		if err != nil {
			t.Fatal(err)
		}
		return auth.NewAuthenticatorWithClientsAndClock(config.Auth{Mode: config.AuthModeBearerStatic}, merged.Keys, func() time.Time { return now })
	}
	check := func(a auth.Authenticator, token string, accepted bool) {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		p, err := a.Authenticate(r)
		if accepted {
			if err != nil || p == nil {
				t.Fatalf("%s: principal = %+v, error = %v", token, p, err)
			}
		} else if p != nil || err != auth.ErrInvalidToken {
			t.Errorf("%s: principal = %+v, error = %v; want nil, ErrInvalidToken", token, p, err)
		}
	}
	live := load()
	for _, offset := range []time.Duration{-time.Nanosecond, 0, time.Nanosecond} {
		now = deadline.Add(offset)
		check(live, rows[0].Name, offset < 0)
		check(live, rows[1].Name, true)
		check(live, rows[2].Name, false)
		check(live, rows[3].Name, false)
	}

	now = deadline.Add(-time.Nanosecond)
	row := rows[0]
	rotatedToken := "rotated-" + suffix
	row.TokenHash = store.TokenHash(rotatedToken)
	if err := st.UpdateInboundKey(ctx, row); err != nil {
		t.Fatal(err)
	}
	rotated := load()
	check(live, row.Name, true)
	check(live, rotatedToken, false)
	check(rotated, row.Name, false)
	check(rotated, rotatedToken, true)
	now = deadline
	check(rotated, rotatedToken, false)

	now = deadline.Add(-time.Nanosecond)
	row.Enabled = false
	if err := st.UpdateInboundKey(ctx, row); err != nil {
		t.Fatal(err)
	}
	check(load(), rotatedToken, false)
	check(rotated, rotatedToken, true)
	if err := st.DeleteInboundKey(ctx, row.ID); err != nil {
		t.Fatal(err)
	}
	check(load(), rotatedToken, false)
	check(rotated, rotatedToken, true)
}
