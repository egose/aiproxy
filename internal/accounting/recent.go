package accounting

import (
	"strings"
	"unicode/utf8"
)

const (
	RecentIdentityBytes    = 256
	RecentModelBytes       = 512
	RecentOperationBytes   = 64
	RecentEffortBytes      = 64
	RecentEntryStringBytes = 4*RecentIdentityBytes + 3*RecentModelBytes + RecentOperationBytes + RecentEffortBytes
	RecentEntryJSONBytes   = 6*RecentEntryStringBytes + 1024
)

type RecentTruncation struct {
	RequestID       bool `json:",omitempty"`
	PublicModel     bool `json:",omitempty"`
	Tenant          bool `json:",omitempty"`
	Client          bool `json:",omitempty"`
	Model           bool `json:",omitempty"`
	Operation       bool `json:",omitempty"`
	Provider        bool `json:",omitempty"`
	UpstreamModel   bool `json:",omitempty"`
	ReasoningEffort bool `json:",omitempty"`
}

func (t RecentTruncation) Fields() []string {
	var fields []string
	for _, field := range []struct {
		name      string
		truncated bool
	}{
		{"RequestID", t.RequestID}, {"PublicModel", t.PublicModel},
		{"Tenant", t.Tenant}, {"Client", t.Client}, {"Model", t.Model},
		{"Operation", t.Operation}, {"Provider", t.Provider}, {"UpstreamModel", t.UpstreamModel},
		{"ReasoningEffort", t.ReasoningEffort},
	} {
		if field.truncated {
			fields = append(fields, field.name)
		}
	}
	return fields
}

func boundedRecent(e Event) Event {
	e.RequestID = recentString(e.RequestID, RecentIdentityBytes, &e.Truncated.RequestID)
	e.PublicModel = recentString(e.PublicModel, RecentModelBytes, &e.Truncated.PublicModel)
	e.Tenant = recentString(e.Tenant, RecentIdentityBytes, &e.Truncated.Tenant)
	e.Client = recentString(e.Client, RecentIdentityBytes, &e.Truncated.Client)
	e.Model = recentString(e.Model, RecentModelBytes, &e.Truncated.Model)
	e.Operation = recentString(e.Operation, RecentOperationBytes, &e.Truncated.Operation)
	e.Provider = recentString(e.Provider, RecentIdentityBytes, &e.Truncated.Provider)
	e.UpstreamModel = recentString(e.UpstreamModel, RecentModelBytes, &e.Truncated.UpstreamModel)
	e.ReasoningEffort = recentString(e.ReasoningEffort, RecentEffortBytes, &e.Truncated.ReasoningEffort)
	return e
}

func recentString(s string, budget int, truncated *bool) string {
	if len(s) > budget {
		end := budget
		for end > 0 && !utf8.RuneStart(s[end]) {
			end--
		}
		s = s[:end]
		*truncated = true
	}
	return strings.Clone(s)
}
