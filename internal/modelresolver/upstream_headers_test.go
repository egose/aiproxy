package modelresolver

import (
	"testing"

	"github.com/egose/aiproxy/internal/config"
)

func TestResolverUpstreamHeadersSnapshot(t *testing.T) {
	rt, err := config.Load([]byte(`
upstream_headers = { x-root = "snapshot" }
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "p" {
  api_key = "fixture"
  model "m" {}
}
`), "upstream-headers.hcl")
	if err != nil {
		t.Fatal(err)
	}
	original := New(rt)
	p, m, ok := original.Model("p", "m")
	if !ok {
		t.Fatal("missing test model")
	}
	got := original.UpstreamHeaders(p, m, nil)
	if got["X-Root"] != "snapshot" {
		t.Fatalf("root headers = %v", got)
	}
	got["X-Root"] = "changed"
	rt.UpstreamHeaders = config.UpstreamHeaders{}
	if next := NewWithPrevious(rt, original).UpstreamHeaders(p, m, nil); len(next) != 0 {
		t.Fatalf("new resolver retained old root headers: %v", next)
	}
	if still := original.UpstreamHeaders(p, m, nil); still["X-Root"] != "snapshot" {
		t.Fatalf("original resolver snapshot changed: %v", still)
	}
}
