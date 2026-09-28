package dashboard

import (
	"testing"

	"github.com/egose/aiproxy/internal/dashrpc"
)

func TestPayloadFilterPendingTransitionKeepsSafeIdentity(t *testing.T) {
	for _, selected := range []string{"removed", "retained"} {
		t.Run(selected, func(t *testing.T) {
			m := requestModel()
			f := payloadTestFetcher()
			f.list = dashrpc.PayloadList{Enabled: true, Payloads: []PayloadSummary{
				{RequestID: "first-error", Status: 500},
				{RequestID: "removed", Status: 200},
				{RequestID: "retained", Status: 502},
			}}
			f.detail["retained"] = "selected retained payload"
			m.payloadFetcher = f
			layoutApplyKey(m, "3")
			layoutApplyKey(m, "down")
			if selected == "retained" {
				layoutApplyKey(m, "down")
			}
			pending := layoutKey(m, "s")
			if pending == nil {
				t.Fatal("filter did not start replacement fetch")
			}
			if cmd := layoutKey(m, "enter"); cmd != nil || m.payloadPendingID != "" {
				t.Fatal("pending filter opened a hidden cached row")
			}
			if m.payloadCursor != 1 || m.payloadAt(m.payloadCursor).RequestID != "retained" {
				t.Fatalf("filter lost retained identity/clamped fallback: cursor=%d", m.payloadCursor)
			}
			m.Update(pending())
			layoutApplyKey(m, "enter")
			if m.payloadDetailID != "retained" || m.payloadDetail != "selected retained payload" {
				t.Fatalf("replacement fetch changed selected detail: %q %q", m.payloadDetailID, m.payloadDetail)
			}
		})
	}
}
