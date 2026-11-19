package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

type directWriteSelection struct {
	query   string
	headers []string
}

func callDirectWrite(h *Handler, ctx context.Context, access, method, path, body string, selection directWriteSelection) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path+selection.query, strings.NewReader(body)).WithContext(ctx)
	r.Header.Set("Authorization", "Bearer "+access)
	r.Header.Set("Content-Type", "application/json")
	for _, value := range selection.headers {
		r.Header.Add("X-Workspace-ID", value)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func deniedDirectWriteSelections(id, other string) []struct {
	name      string
	selection directWriteSelection
	malformed bool
} {
	return []struct {
		name      string
		selection directWriteSelection
		malformed bool
	}{
		{"wrong-query", directWriteSelection{query: "?workspace_id=" + other}, false},
		{"wrong-header", directWriteSelection{headers: []string{other}}, false},
		{"unknown-workspace", directWriteSelection{query: "?workspace_id=" + uuid.NewString()}, false},
		{"wrong-query-matching-header", directWriteSelection{query: "?workspace_id=" + other, headers: []string{id}}, false},
		{"matching-query-wrong-header", directWriteSelection{query: "?workspace_id=" + id, headers: []string{other}}, false},
		{"conflicting-query-repeats", directWriteSelection{query: "?workspace_id=" + id + "&workspace_id=" + other}, false},
		{"conflicting-header-repeats", directWriteSelection{headers: []string{id, other}}, false},
		{"empty-query", directWriteSelection{query: "?workspace_id="}, true},
		{"empty-header", directWriteSelection{headers: []string{" "}}, true},
		{"malformed-query-escape", directWriteSelection{query: "?workspace_id=%zz", headers: []string{id}}, true},
		{"malformed-query-repeat", directWriteSelection{query: "?workspace_id=" + id + "&workspace_id=invalid", headers: []string{id}}, true},
		{"malformed-header-repeat", directWriteSelection{query: "?workspace_id=" + id, headers: []string{id, "invalid"}}, true},
	}
}

func readDirectWriteRevisions(t *testing.T, st *store.Store) store.ServingRevisions {
	t.Helper()
	revisions, err := st.ServingRevisions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return revisions
}

func assertNoDirectWriteMarkers(t *testing.T, headers http.Header) {
	t.Helper()
	for _, name := range []string{catalogSavedHeader, "X-Aiproxy-Key-Saved", "X-Aiproxy-Key-ID", "Location", "X-Aiproxy-Local-Active", "X-Aiproxy-Cluster-Acknowledged", "X-Aiproxy-Applied-Catalog-Revision", "X-Aiproxy-Applied-Key-Revision"} {
		if len(headers.Values(name)) != 0 {
			t.Errorf("unexpected save or activation marker %s", name)
		}
	}
}
