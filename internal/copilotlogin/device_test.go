package copilotlogin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func immediateSleep(context.Context, time.Duration) error { return nil }

func newTestClient(t *testing.T, mux http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(mux)
	c := New()
	c.HTTP = defaultHTTPClient()
	c.DeviceCodeURL = srv.URL + "/login/device/code"
	c.TokenURL = srv.URL + "/login/oauth/access_token"
	c.Sleep = immediateSleep
	return c, srv
}

func TestRequestCodeSuccess(t *testing.T) {
	var gotForm map[string]string
	mux := http.NewServeMux()
	mux.HandleFunc("/login/device/code", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		_ = r.ParseForm()
		gotForm = map[string]string{"client_id": r.Form.Get("client_id"), "scope": r.Form.Get("scope")}
		if r.Form.Get("client_secret") != "" {
			t.Errorf("client_secret must not be sent")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_code": "dev-123", "user_code": "WDXA-XXXX",
			"verification_uri": "https://github.com/login/device",
			"expires_in":       900, "interval": 5,
		})
	})
	c, srv := newTestClient(t, mux)
	defer srv.Close()
	code, err := c.RequestCode(context.Background(), "Ov23test", "read:user")
	if err != nil {
		t.Fatalf("RequestCode(): %v", err)
	}
	if code.DeviceCode != "dev-123" || code.UserCode != "WDXA-XXXX" {
		t.Fatalf("code = %+v", code)
	}
	if code.ExpiresIn != 900*time.Second || code.Interval != 5*time.Second {
		t.Fatalf("expiry/interval = %v/%v", code.ExpiresIn, code.Interval)
	}
	if gotForm["client_id"] != "Ov23test" || gotForm["scope"] != "read:user" {
		t.Fatalf("form = %v", gotForm)
	}
}

func TestRequestCodeDefaults(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/login/device/code", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"device_code":"d","user_code":"u","verification_uri":"https://github.com/login/device"}`))
	})
	c, srv := newTestClient(t, mux)
	defer srv.Close()
	code, err := c.RequestCode(context.Background(), "Ov23test", "")
	if err != nil {
		t.Fatalf("RequestCode(): %v", err)
	}
	if code.ExpiresIn != DefaultExpiry || code.Interval != DefaultInterval {
		t.Fatalf("defaults = %v/%v", code.ExpiresIn, code.Interval)
	}
}

func TestRequestCodeRequiresClientID(t *testing.T) {
	c := New()
	c.Sleep = immediateSleep
	for _, id := range []string{"", "   ", "has space", strings.Repeat("x", 300)} {
		if _, err := c.RequestCode(context.Background(), id, "read:user"); err == nil {
			t.Fatalf("RequestCode(%q) succeeded", id)
		}
	}
}

func TestPollSuccessAfterPending(t *testing.T) {
	var calls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != DeviceGrantType {
			t.Errorf("grant_type = %q", r.Form.Get("grant_type"))
		}
		if r.Form.Get("client_secret") != "" {
			t.Errorf("client_secret must not be sent")
		}
		n := calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n < 3 {
			_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"gho_test","token_type":"bearer","scope":"read:user"}`))
	})
	c, srv := newTestClient(t, mux)
	defer srv.Close()
	got, err := c.Poll(context.Background(), "Ov23test", "dev-123", time.Millisecond, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("Poll(): %v", err)
	}
	if got.AccessToken != "gho_test" {
		t.Fatalf("token = %+v", got)
	}
	if calls.Load() != 3 {
		t.Fatalf("calls = %d", calls.Load())
	}
}

func TestPollRepeatedSlowdownAccumulates(t *testing.T) {
	var sleeps []time.Duration
	mux := http.NewServeMux()
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":"slow_down"}`))
	})
	c, srv := newTestClient(t, mux)
	defer srv.Close()
	base := time.Millisecond
	c.Sleep = func(ctx context.Context, d time.Duration) error {
		sleeps = append(sleeps, d)
		if len(sleeps) >= 3 {
			return context.Canceled
		}
		return nil
	}
	_, err := c.Poll(context.Background(), "Ov23test", "dev-123", base, time.Now().Add(time.Minute))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Poll() = %v", err)
	}
	if len(sleeps) != 3 {
		t.Fatalf("sleeps = %v", sleeps)
	}
	if sleeps[1]-sleeps[0] != SlowDownIncrement || sleeps[2]-sleeps[1] != SlowDownIncrement {
		t.Fatalf("slowdown not cumulative: %v", sleeps)
	}
}

func TestPollSlowDownPrefersServerInterval(t *testing.T) {
	var sleeps []time.Duration
	var calls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			_, _ = w.Write([]byte(`{"error":"slow_down","interval":30}`))
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"gho_x","token_type":"bearer"}`))
	})
	c, srv := newTestClient(t, mux)
	defer srv.Close()
	c.Sleep = func(ctx context.Context, d time.Duration) error {
		sleeps = append(sleeps, d)
		return nil
	}
	_, err := c.Poll(context.Background(), "Ov23test", "dev-123", 5*time.Second, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("Poll(): %v", err)
	}
	if len(sleeps) != 2 || sleeps[0] != 5*time.Second || sleeps[1] != 30*time.Second {
		t.Fatalf("sleeps = %v", sleeps)
	}
}

func TestPollDenial(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":"access_denied"}`))
	})
	c, srv := newTestClient(t, mux)
	defer srv.Close()
	_, err := c.Poll(context.Background(), "Ov23test", "dev-123", time.Millisecond, time.Now().Add(time.Minute))
	if !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("Poll() = %v", err)
	}
}

func TestPollExpiryVariants(t *testing.T) {
	for _, code := range []string{"expired_token", "token_expired"} {
		mux := http.NewServeMux()
		mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fmt.Sprintf(`{"error":%q}`, code)))
		})
		c, srv := newTestClient(t, mux)
		_, err := c.Poll(context.Background(), "Ov23test", "dev-123", time.Millisecond, time.Now().Add(time.Minute))
		srv.Close()
		if !errors.Is(err, ErrExpired) {
			t.Fatalf("Poll(%s) = %v", code, err)
		}
	}
}

func TestPollDeadlineEnforcement(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
	})
	c, srv := newTestClient(t, mux)
	defer srv.Close()
	_, err := c.Poll(context.Background(), "Ov23test", "dev-123", time.Millisecond, time.Now().Add(-time.Second))
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("Poll() = %v, want expired", err)
	}
}

func TestPollCancellation(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
	})
	c, srv := newTestClient(t, mux)
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.Poll(ctx, "Ov23test", "dev-123", time.Second, time.Now().Add(time.Minute))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Poll() = %v", err)
	}
}

func TestPollCancelledDuringSleep(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach server after cancel")
	})
	c, srv := newTestClient(t, mux)
	defer srv.Close()
	c.Sleep = func(ctx context.Context, d time.Duration) error { return context.Canceled }
	_, err := c.Poll(context.Background(), "Ov23test", "dev-123", time.Second, time.Now().Add(time.Minute))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Poll() = %v", err)
	}
}

func TestMalformedResponses(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"not-json", "not json{"},
		{"missing-fields", `{"device_code":""}`},
		{"empty", ``},
		{"token-missing-both", `{"scope":"read:user"}`},
	}
	for _, tc := range cases[:3] {
		mux := http.NewServeMux()
		mux.HandleFunc("/login/device/code", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(tc.body))
		})
		c, srv := newTestClient(t, mux)
		_, err := c.RequestCode(context.Background(), "Ov23test", "read:user")
		srv.Close()
		if err == nil {
			t.Fatalf("%s: expected error", tc.name)
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"scope":"read:user"}`))
	})
	c, srv := newTestClient(t, mux)
	defer srv.Close()
	_, err := c.Poll(context.Background(), "Ov23test", "dev-123", time.Millisecond, time.Now().Add(time.Minute))
	if err == nil {
		t.Fatalf("malformed token: expected error")
	}
}

func TestOversizedResponse(t *testing.T) {
	big := strings.Repeat("x", MaxResponseBytes+100)
	mux := http.NewServeMux()
	mux.HandleFunc("/login/device/code", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"device_code":"` + big + `"}`))
	})
	c, srv := newTestClient(t, mux)
	defer srv.Close()
	_, err := c.RequestCode(context.Background(), "Ov23test", "read:user")
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("RequestCode() = %v, want too large", err)
	}
}

func TestRedirectRefusal(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/login/device/code", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://evil.example/collect?c="+r.FormValue("client_id"), http.StatusFound)
	})
	c, srv := newTestClient(t, mux)
	defer srv.Close()
	_, err := c.RequestCode(context.Background(), "Ov23test", "read:user")
	if !errors.Is(err, ErrRedirect) {
		t.Fatalf("RequestCode() = %v, want redirect refusal", err)
	}
	mux2 := http.NewServeMux()
	mux2.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://evil.example/collect", http.StatusFound)
	})
	c2, srv2 := newTestClient(t, mux2)
	defer srv2.Close()
	_, err = c2.Poll(context.Background(), "Ov23test", "dev-123", time.Millisecond, time.Now().Add(time.Minute))
	if !errors.Is(err, ErrRedirect) {
		t.Fatalf("Poll() = %v, want redirect refusal", err)
	}
}

func TestNetworkFailure(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	c := New()
	c.Sleep = immediateSleep
	c.DeviceCodeURL = url + "/login/device/code"
	c.TokenURL = url + "/login/oauth/access_token"
	if _, err := c.RequestCode(context.Background(), "Ov23test", "read:user"); err == nil {
		t.Fatalf("RequestCode() succeeded")
	} else if errors.Is(err, ErrRedirect) || strings.Contains(err.Error(), "gho_") {
		t.Fatalf("RequestCode() leaked: %v", err)
	}
	if _, err := c.Poll(context.Background(), "Ov23test", "dev", time.Millisecond, time.Now().Add(time.Minute)); err == nil {
		t.Fatalf("Poll() succeeded")
	}
}

func TestProductionEndpointsAreHTTPS(t *testing.T) {
	if !strings.HasPrefix(DeviceCodeURL, "https://github.com/") {
		t.Fatalf("DeviceCodeURL = %q", DeviceCodeURL)
	}
	if !strings.HasPrefix(TokenURL, "https://github.com/") {
		t.Fatalf("TokenURL = %q", TokenURL)
	}
	c := New()
	if c.DeviceCodeURL != DeviceCodeURL || c.TokenURL != TokenURL {
		t.Fatalf("defaults not production: %+v", c)
	}
	if c.HTTP == nil || c.HTTP.Timeout <= 0 {
		t.Fatalf("HTTP timeout not bound")
	}
	if c.HTTP.CheckRedirect == nil {
		t.Fatalf("redirect policy missing")
	}
}

func TestNonLoopbackHTTPRejected(t *testing.T) {
	c := New()
	c.Sleep = immediateSleep
	c.DeviceCodeURL = "http://example.com/login/device/code"
	if _, err := c.RequestCode(context.Background(), "Ov23test", "read:user"); err == nil {
		t.Fatalf("expected endpoint rejection")
	}
}
