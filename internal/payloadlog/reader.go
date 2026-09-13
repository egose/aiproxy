package payloadlog

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/egose/aiproxy/internal/config"
)

var ErrPayloadNotFound = errors.New("payload entry not found")

const (
	defaultPayloadListLimit = 100
	maxPayloadListLimit     = 500
	maxPayloadScanFiles     = 5
	maxPayloadScanLines     = 5000
	maxPayloadGetFiles      = 10
	payloadTailChunkSize    = 64 * 1024
	maxPayloadLineBytes     = 16 << 20
)

type Summary struct {
	Timestamp     string `json:"ts"`
	RequestID     string `json:"request_id,omitempty"`
	Method        string `json:"method,omitempty"`
	Path          string `json:"path,omitempty"`
	Operation     string `json:"operation,omitempty"`
	PublicModel   string `json:"public_model,omitempty"`
	Client        string `json:"client,omitempty"`
	Tenant        string `json:"tenant,omitempty"`
	Provider      string `json:"provider,omitempty"`
	UpstreamModel string `json:"upstream_model,omitempty"`
	Status        int    `json:"status"`
	DurationMs    int64  `json:"duration_ms"`
	Streaming     bool   `json:"streaming,omitempty"`
	Error         string `json:"error,omitempty"`
}

func (s Summary) IsError() bool {
	return s.Status >= 400 || s.Status == 0
}

func payloadFilesNewestFirst(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, config.PayloadLogFilePrefix) || !strings.HasSuffix(name, config.PayloadLogFileExt) {
			continue
		}
		out = append(out, filepath.Join(dir, name))
	}
	sort.Sort(sort.Reverse(sort.StringSlice(out)))
	return out, nil
}

func scanNewestFirst(path string, fn func(line []byte) bool) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	size := st.Size()
	if size == 0 {
		return nil
	}
	var carry []byte
	pos := size
	chunk := make([]byte, payloadTailChunkSize)
	for pos > 0 {
		n := int64(len(chunk))
		if n > pos {
			n = pos
		}
		pos -= n
		if _, err := f.ReadAt(chunk[:n], pos); err != nil {
			return err
		}
		combined := make([]byte, 0, int(n)+len(carry))
		combined = append(combined, chunk[:n]...)
		combined = append(combined, carry...)
		start := len(combined)
		for {
			idx := lastNewline(combined[:start])
			if idx < 0 {
				break
			}
			line := combined[idx+1 : start]
			start = idx
			if len(line) == 0 {
				continue
			}
			if !fn(line) {
				return nil
			}
		}
		carry = append(carry[:0], combined[:start]...)
		if len(carry) > maxPayloadLineBytes {
			return fmt.Errorf("payload line exceeds %d bytes in %s", maxPayloadLineBytes, path)
		}
		if pos == 0 && len(carry) > 0 {
			if !fn(carry) {
				return nil
			}
		}
	}
	return nil
}

func lastNewline(b []byte) int {
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] == '\n' {
			return i
		}
	}
	return -1
}

func ListRecent(dir string, limit int, errorsOnly bool) ([]Summary, error) {
	if limit <= 0 {
		limit = defaultPayloadListLimit
	}
	if limit > maxPayloadListLimit {
		limit = maxPayloadListLimit
	}
	files, err := payloadFilesNewestFirst(dir)
	if err != nil {
		return nil, err
	}
	var out []Summary
	scanned := 0
	for i, path := range files {
		if i >= maxPayloadScanFiles || len(out) >= limit || scanned >= maxPayloadScanLines {
			break
		}
		serr := scanNewestFirst(path, func(line []byte) bool {
			scanned++
			if scanned > maxPayloadScanLines || len(out) >= limit {
				return false
			}
			var s Summary
			if err := json.Unmarshal(line, &s); err != nil {
				return true
			}
			if errorsOnly && !s.IsError() {
				return true
			}
			out = append(out, s)
			return len(out) < limit
		})
		if serr != nil {
			return out, serr
		}
	}
	return out, nil
}

func validRequestID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, r := range id {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func Get(dir, requestID string) (json.RawMessage, error) {
	if !validRequestID(requestID) {
		return nil, fmt.Errorf("invalid request id %q", requestID)
	}
	files, err := payloadFilesNewestFirst(dir)
	if err != nil {
		return nil, err
	}
	var found json.RawMessage
	var probe struct {
		RequestID string `json:"request_id"`
	}
	for i, path := range files {
		if i >= maxPayloadGetFiles || found != nil {
			break
		}
		serr := scanNewestFirst(path, func(line []byte) bool {
			probe.RequestID = ""
			if err := json.Unmarshal(line, &probe); err != nil {
				return true
			}
			if probe.RequestID == requestID {
				found = append(json.RawMessage(nil), line...)
				return false
			}
			return true
		})
		if serr != nil {
			return nil, serr
		}
	}
	if found == nil {
		return nil, ErrPayloadNotFound
	}
	return found, nil
}

func Pretty(raw json.RawMessage, maxBytes int) string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		if maxBytes > 0 && len(raw) > maxBytes {
			return string(raw[:maxBytes]) + "\n…[truncated]"
		}
		return string(raw)
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return string(raw)
	}
	if maxBytes > 0 && len(out) > maxBytes {
		return string(out[:maxBytes]) + "\n…[truncated]"
	}
	return string(out)
}
