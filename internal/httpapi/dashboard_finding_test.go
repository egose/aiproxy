package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/dashrpc"
	"github.com/egose/aiproxy/internal/guardrails"
)

func TestDashboardSelectedHashOperatorBoundaryAndFutureEffect(t *testing.T) {
	for _, action := range []string{"allow", "redact", "deny"} {
		t.Run(action, func(t *testing.T) {
			rt, adapter := newRT(), &countingAdapter{}
			q := guardrails.NewQuarantine(guardrails.QuarantinePolicy{Enabled: true})
			exc := exceptionTestStore(t)
			deps := exceptionTestDeps(t, rt, adapter, guardrailTestScanner(t, guardrails.ModeBlock, 0, 0), q, exc)
			deps.Dashboard = newDashboardDeps(rt, time.Now(), nil, nil, nil).Dashboard
			deps.Dashboard.(*dashrpc.RuntimeSource).SetBlockSource(quarantineTestAdapter{q})
			h := NewHandler(deps)
			secret := guardrailTestKey(t)
			blocked := serveChat(t, h, "deploy with "+secret)
			id := blockIDOf(t, blocked.Body.String())
			capture, ok := q.Take(id)
			if !ok || len(capture.Findings) == 0 || strings.Contains(blocked.Body.String(), secret) {
				t.Fatal("initial block did not preserve confidential capture")
			}
			selectedSHA := capture.Findings[0].SecretSHA
			otherSecret := "other-unselected-fixture"
			otherSHA := guardrails.Fingerprint(otherSecret)
			capture.Findings = append(capture.Findings, guardrails.CapturedFinding{RuleID: "other", Secret: otherSecret, SecretSHA: otherSHA})
			q.Store(id, capture)
			if _, err := exc.Decide([]string{otherSHA}, "deny", id, nil); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(exc.Path())
			if err != nil {
				t.Fatal(err)
			}
			denials := 0
			for _, token := range []string{"", "Bearer invalid", "Bearer ordinary-api-key"} {
				for _, method := range []string{http.MethodGet, http.MethodPost} {
					path := dashrpc.BlockPathPrefix + id
					if method == http.MethodPost {
						path = dashrpc.BlockDecisionPath(id)
					}
					request := httptest.NewRequest(method, path, strings.NewReader(`{"action":"allow","finding_shas":["`+selectedSHA+`"]}`))
					request.Header.Set("Authorization", token)
					recorder := httptest.NewRecorder()
					h.ServeHTTP(recorder, request)
					denials++
					want := http.StatusUnauthorized
					if denials > dashboardAuthBurst {
						want = http.StatusTooManyRequests
					}
					if recorder.Code != want || strings.Contains(recorder.Body.String(), secret) || strings.Contains(recorder.Body.String(), otherSecret) || q.Len() != 1 {
						t.Fatalf("denied operator access leaked/consumed capture: %d", recorder.Code)
					}
				}
			}
			after, err := os.ReadFile(exc.Path())
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("denied operator decision changed persistent state")
			}
			server := httptest.NewServer(h)
			defer server.Close()
			client := dashrpc.NewClient(server.URL, dashboardTestToken)
			defer client.HTTP.CloseIdleConnections()
			list, err := client.FetchBlocks(context.Background())
			if err != nil || len(list.Blocks) != 1 {
				t.Fatalf("list: %+v %v", list, err)
			}
			raw, _ := json.Marshal(list)
			if strings.Contains(string(raw), secret) || strings.Contains(string(raw), otherSecret) {
				t.Fatal("summary exposed body")
			}
			detail, err := client.FetchBlock(context.Background(), id)
			if err != nil || len(detail.Findings) != len(capture.Findings) || detail.Findings[0].SecretSHA != selectedSHA || q.Len() != 0 {
				t.Fatalf("operator take: %+v %v", detail, err)
			}
			if _, err := client.FetchBlock(context.Background(), id); err == nil {
				t.Fatal("take-once capture reopened")
			}
			ack, err := client.DecideBlock(context.Background(), id, action, []string{detail.Findings[0].SecretSHA})
			if err != nil || !ack.Ok || ack.Action != action || ack.Count != 1 {
				t.Fatalf("decision: %+v %v", ack, err)
			}
			if adapter.count() != 0 {
				t.Fatal("decision replayed original request")
			}
			reloaded, err := guardrails.LoadExceptions(exc.Path(), "")
			if err != nil {
				t.Fatal(err)
			}
			for sha, want := range map[string]string{selectedSHA: action, otherSHA: "deny"} {
				if decision, ok := reloaded.Lookup(sha); !ok || decision.Action != want {
					t.Fatalf("persistent exact-hash scope: %q = %+v %v", sha, decision, ok)
				}
			}
			raw, err = os.ReadFile(exc.Path())
			if err != nil || strings.Contains(string(raw), secret) || strings.Contains(string(raw), otherSecret) {
				t.Fatal("persistent decisions retained secret body")
			}
			future := serveChat(t, h, "deploy with "+secret)
			if action == "deny" {
				if future.Code != http.StatusBadRequest || adapter.count() != 0 {
					t.Fatal("deny failed to block future match")
				}
			} else if future.Code != http.StatusOK || adapter.count() != 1 {
				t.Fatalf("future request not admitted: %d", future.Code)
			}
			if action == "redact" && (strings.Contains(string(adapter.lastBody()), secret) || !strings.Contains(string(adapter.lastBody()), guardrails.DefaultRedactPlaceholder)) {
				t.Fatal("future redact did not replace selected secret")
			}
		})
	}
}
