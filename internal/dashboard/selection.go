package dashboard

import (
	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/config"
)

func anchoredIndex[T any, K comparable](before, after []T, index int, key func(T) K) int {
	if index >= 0 && index < len(before) {
		wanted := key(before[index])
		for i, row := range after {
			if key(row) == wanted {
				return i
			}
		}
	}
	return max(0, min(index, len(after)-1))
}

func providerIdentity(p config.Provider) string { return p.Name }
func aliasIdentity(a config.Alias) string       { return a.Name }
func payloadIdentity(p PayloadSummary) string   { return p.RequestID }
func blockIdentity(b BlockSummary) string       { return b.BlockID }

func usageIdentity(s accounting.Summary) accounting.Summary {
	return accounting.Summary{Tenant: s.Tenant, Client: s.Client, Model: s.Model, Operation: s.Operation, StatusCode: s.StatusCode}
}

func (m *model) providerList() []config.Provider {
	if m.snapshot == nil {
		return nil
	}
	out := append([]config.Provider(nil), m.snapshot.Providers...)
	return append(out, m.snapshot.DisabledProviders...)
}
