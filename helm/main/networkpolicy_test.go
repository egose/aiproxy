package helm_test

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type policySelector struct {
	MatchLabels map[string]string `yaml:"matchLabels"`
}

type policyPeer struct {
	PodSelector       *policySelector `yaml:"podSelector"`
	NamespaceSelector *policySelector `yaml:"namespaceSelector"`
	IPBlock           *struct {
		CIDR string `yaml:"cidr"`
	} `yaml:"ipBlock"`
}

type policyPort struct {
	Protocol string `yaml:"protocol"`
	Port     int    `yaml:"port"`
}

type policyRule struct {
	From  []policyPeer `yaml:"from"`
	To    []policyPeer `yaml:"to"`
	Ports []policyPort `yaml:"ports"`
}

type policyDocument struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		PodSelector policySelector `yaml:"podSelector"`
		PolicyTypes []string       `yaml:"policyTypes"`
		Ingress     []policyRule   `yaml:"ingress"`
		Egress      []policyRule   `yaml:"egress"`
	} `yaml:"spec"`
}

func renderPolicies(t *testing.T, settings ...string) map[string]policyDocument {
	t.Helper()
	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("Helm is required; run make helm-test with Helm installed")
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "templates"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Chart.yaml"), []byte("apiVersion: v2\nname: main\nversion: 0.1.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"values.yaml", "templates/_helpers.tpl", "templates/networkpolicy.yaml"} {
		data, err := os.ReadFile(filepath.FromSlash(path))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(path)), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	args := []string{"template", "review", dir}
	for _, setting := range settings {
		args = append(args, "--set", setting)
	}
	data, err := exec.Command(helm, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, data)
	}
	policies := map[string]policyDocument{}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	for {
		var doc policyDocument
		if err := decoder.Decode(&doc); err != nil {
			if err == io.EOF {
				break
			}
			t.Fatal(err)
		}
		if doc.Kind != "NetworkPolicy" {
			continue
		}
		policies[strings.TrimPrefix(doc.Metadata.Name, "review-main-")] = doc
	}
	return policies
}

func TestNetworkPolicyIngressRequiresExplicitSources(t *testing.T) {
	policies := renderPolicies(t, "relay.enabled=true")
	for _, name := range []string{"api-ingress", "relay-ingress"} {
		if len(policies[name].Spec.Ingress) != 0 {
			t.Fatalf("%s must not permit ingress without configured peers", name)
		}
	}
	deny := policies["default-deny"]
	if !reflect.DeepEqual(deny.Spec.PolicyTypes, []string{"Ingress", "Egress"}) || !reflect.DeepEqual(deny.Spec.PodSelector.MatchLabels, map[string]string{"app.kubernetes.io/instance": "review"}) {
		t.Fatalf("default deny changed: %+v", deny.Spec)
	}
}

func TestNetworkPolicyExplicitIngressScopes(t *testing.T) {
	cases := []struct {
		name     string
		settings []string
		peer     policyPeer
	}{
		{"same release", []string{"networkPolicy.allowSameNamespace=true"}, policyPeer{PodSelector: &policySelector{MatchLabels: map[string]string{"app.kubernetes.io/instance": "review"}}}},
		{"openshift", []string{"networkPolicy.allowOpenShiftIngress=true"}, policyPeer{NamespaceSelector: &policySelector{MatchLabels: map[string]string{"network.openshift.io/policy-group": "ingress"}}}},
		{"explicit pod", []string{"networkPolicy.api.ingressFrom[0].podSelector.matchLabels.role=caller", "networkPolicy.relay.ingressFrom[0].podSelector.matchLabels.role=caller"}, policyPeer{PodSelector: &policySelector{MatchLabels: map[string]string{"role": "caller"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			policies := renderPolicies(t, append([]string{"relay.enabled=true"}, tc.settings...)...)
			for _, name := range []string{"api-ingress", "relay-ingress"} {
				rules := policies[name].Spec.Ingress
				if len(rules) != 1 || !reflect.DeepEqual(rules[0].From, []policyPeer{tc.peer}) {
					t.Fatalf("%s peer scope changed: %+v", name, rules)
				}
			}
		})
	}
}

func TestNetworkPolicyRelayAPIConnectionRequiresOptIn(t *testing.T) {
	cases := []struct {
		name      string
		settings  []string
		allowed   bool
		apiName   string
		relayName string
		port      int
	}{
		{"default off", nil, false, "api", "relay", 8080},
		{"opt in", []string{"networkPolicy.relay.allowApiUpstream=true"}, true, "api", "relay", 8080},
		{"api disabled", []string{"networkPolicy.relay.allowApiUpstream=true", "api.enabled=false"}, false, "api", "relay", 8080},
		{"relay disabled", []string{"networkPolicy.relay.allowApiUpstream=true", "relay.enabled=false"}, false, "api", "relay", 8080},
		{"overrides", []string{"networkPolicy.relay.allowApiUpstream=true", "api.nameOverride=backend", "relay.nameOverride=edge", "api.service.port=9090"}, true, "backend", "edge", 9090},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			policies := renderPolicies(t, append([]string{"relay.enabled=true"}, tc.settings...)...)
			apiIngress := policies["api-ingress"].Spec.Ingress
			wantAPI := map[string]string{"app.kubernetes.io/name": tc.apiName, "app.kubernetes.io/instance": "review"}
			wantRelay := map[string]string{"app.kubernetes.io/name": tc.relayName, "app.kubernetes.io/instance": "review"}
			wantPort := []policyPort{{Protocol: "TCP", Port: tc.port}}
			if tc.allowed {
				if len(apiIngress) != 1 || !reflect.DeepEqual(apiIngress[0].From, []policyPeer{{PodSelector: &policySelector{MatchLabels: wantRelay}}}) || !reflect.DeepEqual(apiIngress[0].Ports, wantPort) {
					t.Fatalf("API ingress does not restrict access to the relay: %+v", apiIngress)
				}
			} else if len(apiIngress) != 0 {
				t.Fatal("API ingress was opened without the enabled-component opt-in")
			}
			found := false
			for _, rule := range policies["relay-egress"].Spec.Egress {
				if len(rule.To) == 1 && rule.To[0].NamespaceSelector == nil && rule.To[0].PodSelector != nil && reflect.DeepEqual(rule.To[0].PodSelector.MatchLabels, wantAPI) {
					found = true
					if !reflect.DeepEqual(rule.Ports, wantPort) {
						t.Fatalf("relay API egress port changed: %+v", rule.Ports)
					}
				}
			}
			if found != tc.allowed {
				t.Fatalf("relay API egress allowed=%t, want %t", found, tc.allowed)
			}
			if len(policies["relay-ingress"].Spec.Ingress) != 0 {
				t.Fatal("upstream opt-in must not open relay ingress")
			}
		})
	}
}

func TestNetworkPolicyRetainsDNSAndExplicitEgress(t *testing.T) {
	policies := renderPolicies(t,
		"relay.enabled=true",
		"networkPolicy.relay.upstreamEgressIpBlocks[0].cidr=203.0.113.20/32",
		"networkPolicy.relay.extraEgress[0].to[0].podSelector.matchLabels.role=upstream",
		"networkPolicy.relay.extraEgress[0].ports[0].protocol=TCP",
		"networkPolicy.relay.extraEgress[0].ports[0].port=8081",
	)
	rules := policies["relay-egress"].Spec.Egress
	if len(rules) != 3 || !reflect.DeepEqual(rules[0].Ports, []policyPort{{Protocol: "UDP", Port: 53}, {Protocol: "TCP", Port: 53}}) {
		t.Fatalf("DNS or explicit egress rules changed: %+v", rules)
	}
	if len(rules[0].To) != 1 || rules[0].To[0].NamespaceSelector == nil || rules[0].To[0].PodSelector == nil {
		t.Fatal("DNS selectors are missing")
	}
	if len(rules[1].To) != 1 || rules[1].To[0].IPBlock == nil || rules[1].To[0].IPBlock.CIDR != "203.0.113.20/32" || !reflect.DeepEqual(rules[1].Ports, []policyPort{{Protocol: "TCP", Port: 443}}) {
		t.Fatal("HTTPS CIDR egress was not preserved")
	}
	if len(rules[2].To) != 1 || rules[2].To[0].PodSelector == nil || rules[2].To[0].PodSelector.MatchLabels["role"] != "upstream" || !reflect.DeepEqual(rules[2].Ports, []policyPort{{Protocol: "TCP", Port: 8081}}) {
		t.Fatal("extra egress was not preserved")
	}
}

func TestNetworkPolicyCanBeDisabled(t *testing.T) {
	if policies := renderPolicies(t, "relay.enabled=true", "networkPolicy.enabled=false"); len(policies) != 0 {
		t.Fatal("disabled NetworkPolicy still renders resources")
	}
}
