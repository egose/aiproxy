package observability

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/identity"
	"github.com/google/uuid"
)

func TestUsageMetricsDoNotLabelImmutableCredentialsOrWorkspaces(t *testing.T) {
	m := NewMetrics()
	var identifiers []string
	for range 20 {
		key, workspace := uuid.New(), uuid.New()
		identifiers = append(identifiers, key.String(), workspace.String())
		m.Record(accounting.Event{CredentialID: identity.DatabaseKeyID(key), WorkspaceScope: identity.InWorkspace(workspace),
			Tenant: "shared", Client: "shared", Model: "alias/a", Operation: "chat_completions", StatusCode: 200})
	}
	w := httptest.NewRecorder()
	m.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	body := w.Body.String()
	if w.Code != 200 || strings.Count(body, "aiproxy_usage_events_total{") != 1 ||
		!strings.Contains(body, `aiproxy_usage_events_total{client="shared",model="alias/a",operation="chat_completions",status="200",tenant="shared"} 20`) {
		t.Fatalf("immutable identity expanded usage metrics: %s", body)
	}
	for _, forbidden := range append(identifiers, "credential_id", "key_id", "workspace_id", "CredentialID", "WorkspaceScope") {
		if strings.Contains(body, forbidden) {
			t.Fatalf("internal accounting identity in metrics: %q", forbidden)
		}
	}
}
