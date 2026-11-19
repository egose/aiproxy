package accounting

import "github.com/egose/aiproxy/internal/identity"

type Filter struct {
	all          bool
	credential   identity.CredentialID
	workspace    identity.WorkspaceScope
	staticTenant string
	tenantless   bool
}

func AllUsage() Filter {
	return Filter{all: true}
}

func CredentialUsage(credential identity.CredentialID, workspace identity.WorkspaceScope) Filter {
	return Filter{credential: credential, workspace: workspace}
}

func StaticTenantUsage(tenant string, workspace identity.WorkspaceScope) Filter {
	return Filter{staticTenant: tenant, workspace: workspace}
}

func StaticClientUsage(credential identity.CredentialID, workspace identity.WorkspaceScope) Filter {
	return Filter{credential: credential, workspace: workspace, tenantless: true}
}

func (f Filter) matches(credential identity.CredentialID, workspace identity.WorkspaceScope, tenant string) bool {
	if f.all {
		return true
	}
	if _, scoped := f.workspace.ID(); !scoped || workspace != f.workspace {
		return false
	}
	if f.staticTenant != "" {
		_, static := credential.StaticClientName()
		return static && tenant == f.staticTenant
	}
	if f.tenantless {
		if _, static := credential.StaticClientName(); !static || tenant != "" {
			return false
		}
	}
	return f.credential.Valid() && credential == f.credential
}

func FilterSummaries(summaries []Summary, filter Filter) []Summary {
	out := make([]Summary, 0, len(summaries))
	for _, summary := range summaries {
		if filter.matches(summary.CredentialID, summary.WorkspaceScope, summary.Tenant) {
			out = append(out, summary)
		}
	}
	return out
}

func FilterUpstreamSummaries(summaries []UpstreamSummary, filter Filter) []UpstreamSummary {
	out := make([]UpstreamSummary, 0, len(summaries))
	for _, summary := range summaries {
		if filter.matches(summary.CredentialID, summary.WorkspaceScope, summary.Tenant) {
			out = append(out, summary)
		}
	}
	return out
}

func identityLess(a identity.CredentialID, aw identity.WorkspaceScope, b identity.CredentialID, bw identity.WorkspaceScope) bool {
	if aw != bw {
		aid, aScoped := aw.ID()
		bid, bScoped := bw.ID()
		if aScoped != bScoped {
			return !aScoped
		}
		return aid.String() < bid.String()
	}
	if a.Valid() != b.Valid() {
		return !a.Valid()
	}
	an, aStatic := a.StaticClientName()
	bn, bStatic := b.StaticClientName()
	if aStatic != bStatic {
		return aStatic
	}
	if aStatic {
		return an < bn
	}
	aid, _ := a.DatabaseKeyID()
	bid, _ := b.DatabaseKeyID()
	return aid.String() < bid.String()
}
