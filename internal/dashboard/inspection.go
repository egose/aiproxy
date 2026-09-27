package dashboard

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egose/aiproxy/internal/dashrpc"
	"github.com/egose/aiproxy/internal/payloadlog"
)

func inspectionText(s string) string {
	var out strings.Builder
	for len(s) > 0 {
		r, n := utf8.DecodeRuneInString(s)
		if r == utf8.RuneError && n == 1 {
			fmt.Fprintf(&out, "\\x%02x", s[0])
		} else if r == '\n' || !unicode.IsControl(r) {
			out.WriteRune(r)
		} else {
			quoted := strconv.QuoteRune(r)
			out.WriteString(quoted[1 : len(quoted)-1])
		}
		s = s[n:]
	}
	return out.String()
}

func renderInspection(width, height int, headers []string, body, notice string, scroll *int) string {
	frame, inner := paneBox(width, height, true)
	parts := wrapText(inspectionText(body), inner)
	visible := max(1, height-3-len(headers))
	*scroll = clampInt(*scroll, 0, max(0, len(parts)-visible))
	end := min(len(parts), *scroll+visible)
	rows := make([]string, 0, height-2)
	for _, h := range headers {
		rows = append(rows, truncate(logOneLine(inspectionText(h)), inner))
	}
	rows = append(rows, parts[*scroll:end]...)
	for len(rows) < height-3 {
		rows = append(rows, "")
	}
	rows = append(rows, fmt.Sprintf("wrap %d-%d/%d · %s", *scroll+1, end, len(parts), notice))
	return frame.Render(strings.Join(rows, "\n"))
}

func boundedPayloadText(s string) (string, bool) {
	if len(s) <= payloadDetailMaxOut {
		return s, false
	}
	return s[:payloadDetailMaxOut], true
}

func payloadCaptureNotice(s string) string {
	var record payloadlog.Entry
	if json.Unmarshal([]byte(s), &record) != nil {
		return "capture truncation unknown"
	}
	if record.Request.Body.Truncated || record.UpstreamRequest.Body.Truncated || record.Response.Body.Truncated {
		return "TRUNCATED capture (server body cap)"
	}
	return "capture truncation not reported"
}

func validFindingSHA(sha string) bool {
	if len(sha) != 64 || strings.ToLower(sha) != sha {
		return false
	}
	_, err := hex.DecodeString(sha)
	return err == nil
}

func (m *model) selectedBlockFinding() (dashrpc.BlockFinding, bool) {
	if m.blockFindingCursor < 0 || m.blockFindingCursor >= len(m.blockDetail.Findings) {
		return dashrpc.BlockFinding{}, false
	}
	return m.blockDetail.Findings[m.blockFindingCursor], true
}

func (m *model) selectedDecisionStatus() {
	m.blockDecisionMsg, m.blockDecisionErr = "", ""
	f, ok := m.selectedBlockFinding()
	if !ok {
		return
	}
	if result, ok := m.blockDecisionResults[f.SecretSHA]; ok {
		if result.err != "" {
			m.blockDecisionErr = result.err
		} else {
			m.blockDecisionMsg = result.action + " recorded for selected hash"
		}
	}
}

func (m *model) blockInspectionStatus() string {
	switch {
	case m.blockPendingID != "":
		return "take-once read pending; cancellation may still consume capture"
	case m.blockDetailErr != "":
		return "take-once read failed; capture may be expired or already consumed"
	case m.blockDecisionPending != "":
		return "take-once consumed · recording " + m.blockDecisionPending + "; finding locked"
	case m.blockDecisionErr != "":
		return "decision failed; outcome unknown · a/s/d deliberately retries selected hash"
	case m.blockDecisionMsg != "":
		return "take-once consumed · " + m.blockDecisionMsg
	case m.paused:
		return "take-once consumed · PAUSED; p resumes decisions"
	}
	f, ok := m.selectedBlockFinding()
	if !ok {
		return "take-once consumed · no findings; decisions unavailable"
	}
	if !validFindingSHA(f.SecretSHA) {
		return "take-once consumed · missing/invalid SHA; decisions unavailable"
	}
	return "take-once consumed · a allow non-secret / s redact / d deny"
}
