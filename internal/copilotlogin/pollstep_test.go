package copilotlogin

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestPollOneStepPendingAndSlowDown(t *testing.T) {
	var calls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
			return
		}
		_, _ = w.Write([]byte(`{"error":"slow_down","interval":9}`))
	})
	c, srv := newTestClient(t, mux)
	defer srv.Close()
	first, err := c.PollOneStep(context.Background(), "Ov23test", "dev-123", 5*time.Second)
	if err != nil {
		t.Fatalf("pending step: %v", err)
	}
	if first.State != PollStepPending || first.Interval != 5*time.Second {
		t.Fatalf("pending = %+v", first)
	}
	second, err := c.PollOneStep(context.Background(), "Ov23test", "dev-123", 5*time.Second)
	if err != nil {
		t.Fatalf("slow_down step: %v", err)
	}
	if second.State != PollStepPending || second.Interval != 10*time.Second {
		t.Fatalf("slow_down must not lower advice: %+v", second)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls = %d, want exactly 2 one-step requests", calls.Load())
	}
}

func TestPollOneStepTerminalMapping(t *testing.T) {
	for _, tc := range []struct {
		name  string
		body  string
		state PollStepState
	}{
		{"denied", `{"error":"access_denied"}`, PollStepDenied},
		{"expired", `{"error":"expired_token"}`, PollStepExpired},
		{"expired_alias", `{"error":"token_expired"}`, PollStepExpired},
		{"other", `{"error":"incorrect_device_code"}`, PollStepFailed},
		{"malformed", `{"unexpected":true}`, PollStepFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			})
			c, srv := newTestClient(t, mux)
			defer srv.Close()
			got, err := c.PollOneStep(context.Background(), "Ov23test", "dev-123", 5*time.Second)
			if err == nil {
				t.Fatalf("terminal step must return an error")
			}
			if got.State != tc.state {
				t.Fatalf("state = %q, want %q", got.State, tc.state)
			}
		})
	}
}

func TestPollOneStepReady(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "gho_step", "token_type": "bearer", "scope": "read:user",
		})
	})
	c, srv := newTestClient(t, mux)
	defer srv.Close()
	got, err := c.PollOneStep(context.Background(), "Ov23test", "dev-123", 5*time.Second)
	if err != nil {
		t.Fatalf("ready step: %v", err)
	}
	if got.State != PollStepReady || got.Token.AccessToken != "gho_step" {
		t.Fatalf("ready = %+v", got)
	}
}

func TestPollLoopPreservesSlowDownGrowth(t *testing.T) {
	var calls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n < 3 {
			_, _ = w.Write([]byte(`{"error":"slow_down"}`))
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"gho_loop","token_type":"bearer","scope":"read:user"}`))
	})
	c, srv := newTestClient(t, mux)
	defer srv.Close()
	got, err := c.Poll(context.Background(), "Ov23test", "dev-123", time.Millisecond, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("Poll(): %v", err)
	}
	if got.AccessToken != "gho_loop" || calls.Load() != 3 {
		t.Fatalf("Poll = %+v calls=%d", got, calls.Load())
	}
}
