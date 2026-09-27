package config

import "testing"

func TestDynamicCopilotMaterializedCredential(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		p := Provider{Type: ProviderTypeGitHubCopilot, Name: "copilot", Enabled: enabled, CopilotToken: "database-token"}
		if err := validateDynamicCredential(p); err != nil {
			t.Fatal(err)
		}
		p.CopilotCredentialRef = &CopilotCredentialRef{Name: "sidecar", Resolved: true}
		if err := validateDynamicCredential(p); err != nil {
			t.Fatal("resolved sidecar plus token is legitimate", err)
		}
		p.APIKey = "api-key"
		if err := validateDynamicCredential(p); err == nil {
			t.Fatal("mixed API key accepted")
		}
		p.APIKey = ""
		p.Type = ProviderTypeOpenAI
		p.CopilotCredentialRef = nil
		if err := validateDynamicCredential(p); err == nil {
			t.Fatal("dedicated token accepted for wrong type")
		}
	}
}
