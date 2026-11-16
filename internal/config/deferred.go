package config

import (
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

func DefaultDeferred() Deferred {
	return Deferred{
		MaxJobs:             64,
		MaxRunning:          8,
		MaxAdmissions:       4,
		MaxEntries:          256,
		MaxMemoryBytes:      256 << 20,
		MaxRequestBytes:     1 << 20,
		MaxResultBytes:      8 << 20,
		MaxWorkBytes:        64 << 20,
		PerOwnerJobs:        8,
		PerOwnerRunning:     2,
		MaxControlInflight:  16,
		ExecutionTimeout:    10 * time.Minute,
		Retention:           2 * time.Minute,
		ControlTimeout:      5 * time.Second,
		FinalizationTimeout: 5 * time.Second,
	}
}

func DefaultDeferredTransport() DeferredTransport {
	return DeferredTransport{
		ControlTimeout:    5 * time.Second,
		MaxWait:           15 * time.Minute,
		PollInterval:      250 * time.Millisecond,
		MaxControlCalls:   4096,
		MaxControlRetries: 8,
	}
}

func buildDeferred(raw *rawDeferred) (Deferred, error) {
	out := DefaultDeferred()
	out.Enabled = true
	for _, field := range []struct {
		raw *int
		out *int
	}{
		{raw.MaxJobs, &out.MaxJobs},
		{raw.MaxRunning, &out.MaxRunning},
		{raw.MaxAdmissions, &out.MaxAdmissions},
		{raw.MaxEntries, &out.MaxEntries},
		{raw.PerOwnerJobs, &out.PerOwnerJobs},
		{raw.PerOwnerRunning, &out.PerOwnerRunning},
		{raw.MaxControlInflight, &out.MaxControlInflight},
	} {
		if field.raw != nil {
			*field.out = *field.raw
		}
	}
	for _, field := range []struct {
		raw *int64
		out *int64
	}{
		{raw.MaxMemoryBytes, &out.MaxMemoryBytes},
		{raw.MaxRequestBytes, &out.MaxRequestBytes},
		{raw.MaxResultBytes, &out.MaxResultBytes},
		{raw.MaxWorkBytes, &out.MaxWorkBytes},
	} {
		if field.raw != nil {
			*field.out = *field.raw
		}
	}
	for _, field := range []struct {
		name string
		raw  *string
		out  *time.Duration
		min  time.Duration
		max  time.Duration
	}{
		{"execution_timeout", raw.ExecutionTimeout, &out.ExecutionTimeout, time.Second, time.Hour},
		{"retention", raw.Retention, &out.Retention, 10 * time.Second, time.Hour},
		{"control_timeout", raw.ControlTimeout, &out.ControlTimeout, 100 * time.Millisecond, 30 * time.Second},
		{"finalization_timeout", raw.FinalizationTimeout, &out.FinalizationTimeout, 100 * time.Millisecond, 30 * time.Second},
	} {
		if field.raw == nil {
			continue
		}
		value, err := parseDeferredDuration("deferred."+field.name, *field.raw, field.min, field.max)
		if err != nil {
			return Deferred{}, err
		}
		*field.out = value
	}
	if err := ValidateDeferred(out); err != nil {
		return Deferred{}, err
	}
	return out, nil
}

func buildProviderDeferred(p *Provider, raw rawProvider, root Deferred) error {
	if raw.DeferredTransport != nil {
		out := DefaultDeferredTransport()
		for _, field := range []struct {
			name string
			raw  *string
			out  *time.Duration
			min  time.Duration
			max  time.Duration
		}{
			{"control_timeout", raw.DeferredTransport.ControlTimeout, &out.ControlTimeout, 100 * time.Millisecond, 30 * time.Second},
			{"max_wait", raw.DeferredTransport.MaxWait, &out.MaxWait, time.Second, 2 * time.Hour},
			{"poll_interval", raw.DeferredTransport.PollInterval, &out.PollInterval, 50 * time.Millisecond, 5 * time.Second},
		} {
			if field.raw == nil {
				continue
			}
			value, err := parseDeferredDuration("deferred_transport."+field.name, *field.raw, field.min, field.max)
			if err != nil {
				return err
			}
			*field.out = value
		}
		if raw.DeferredTransport.MaxControlCalls != nil {
			out.MaxControlCalls = *raw.DeferredTransport.MaxControlCalls
		}
		if raw.DeferredTransport.MaxControlRetries != nil {
			out.MaxControlRetries = *raw.DeferredTransport.MaxControlRetries
		}
		p.DeferredTransport = &out
	}
	if raw.DeferredReceiver != nil {
		out := DeferredReceiver{ExecutionTimeout: root.ExecutionTimeout, MaxResultBytes: root.MaxResultBytes}
		if raw.DeferredReceiver.ExecutionTimeout != nil {
			value, err := parseDeferredDuration("deferred_receiver.execution_timeout", *raw.DeferredReceiver.ExecutionTimeout, time.Second, root.ExecutionTimeout)
			if err != nil {
				return err
			}
			out.ExecutionTimeout = value
		}
		if raw.DeferredReceiver.MaxResultBytes != nil {
			out.MaxResultBytes = *raw.DeferredReceiver.MaxResultBytes
		}
		p.DeferredReceiver = &out
	}
	return validateProviderDeferred(*p, root)
}

func parseDeferredDuration(field, value string, min, max time.Duration) (time.Duration, error) {
	d, err := time.ParseDuration(value)
	if err != nil || d < min || d > max {
		return 0, fmt.Errorf("%s must be a duration between %s and %s", field, min, max)
	}
	return d, nil
}

func ValidateDeferred(d Deferred) error {
	for _, field := range []struct {
		name string
		val  int64
		min  int64
		max  int64
	}{
		{"max_jobs", int64(d.MaxJobs), 1, 4096},
		{"max_running", int64(d.MaxRunning), 1, 256},
		{"max_admissions", int64(d.MaxAdmissions), 1, 64},
		{"max_entries", int64(d.MaxEntries), int64(d.MaxJobs), 65536},
		{"max_memory_bytes", d.MaxMemoryBytes, 16 << 20, 4 << 30},
		{"max_request_bytes", d.MaxRequestBytes, 1 << 10, 8 << 20},
		{"max_result_bytes", d.MaxResultBytes, 1 << 10, 32 << 20},
		{"max_work_bytes", d.MaxWorkBytes, 4 << 20, 256 << 20},
		{"per_owner_jobs", int64(d.PerOwnerJobs), 1, int64(d.MaxJobs)},
		{"per_owner_running", int64(d.PerOwnerRunning), 1, int64(d.MaxRunning)},
		{"max_control_inflight", int64(d.MaxControlInflight), 4, 128},
	} {
		if field.val < field.min || field.val > field.max {
			return fmt.Errorf("deferred.%s must be between %d and %d", field.name, field.min, field.max)
		}
	}
	if d.MaxRunning > d.MaxJobs {
		return fmt.Errorf("deferred.max_running must not exceed max_jobs")
	}
	for _, field := range []struct {
		name string
		val  time.Duration
		min  time.Duration
		max  time.Duration
	}{
		{"execution_timeout", d.ExecutionTimeout, time.Second, time.Hour},
		{"retention", d.Retention, 10 * time.Second, time.Hour},
		{"control_timeout", d.ControlTimeout, 100 * time.Millisecond, 30 * time.Second},
		{"finalization_timeout", d.FinalizationTimeout, 100 * time.Millisecond, 30 * time.Second},
	} {
		if field.val < field.min || field.val > field.max {
			return fmt.Errorf("deferred.%s must be between %s and %s", field.name, field.min, field.max)
		}
	}
	required, err := deferredMinimumMemory(d)
	if err != nil {
		return err
	}
	if d.MaxMemoryBytes < required {
		return fmt.Errorf("deferred.max_memory_bytes must cover fixed allowances, one maximum request/result/work reservation, and admission scratch (at least %d bytes)", required)
	}
	return nil
}

func deferredMinimumMemory(d Deferred) (int64, error) {
	resultBytes := ((d.MaxResultBytes + (32 << 10) - 1) / (32 << 10)) * (32 << 10)
	submitScratch := ((d.MaxRequestBytes + 2) / 3 * 4) + (16 << 10) + d.MaxRequestBytes
	required := int64(0)
	for _, allowance := range []struct {
		count int64
		bytes int64
	}{
		{int64(d.MaxControlInflight), 512 << 10},
		{int64(d.MaxJobs), 64 << 10},
		{int64(d.MaxEntries), 4 << 10},
		{int64(d.MaxEntries), 1 << 10},
		{int64(d.MaxAdmissions), submitScratch},
		{1, d.MaxRequestBytes},
		{1, resultBytes},
		{1, d.MaxWorkBytes},
	} {
		if allowance.count < 0 || allowance.bytes < 0 || (allowance.count != 0 && allowance.bytes > (math.MaxInt64-required)/allowance.count) {
			return 0, fmt.Errorf("deferred memory allowances overflow int64")
		}
		required += allowance.count * allowance.bytes
	}
	return required, nil
}

func validateProviderDeferred(p Provider, root Deferred) error {
	if p.DeferredTransport == nil && p.DeferredReceiver == nil {
		return nil
	}
	if p.DeferredTransport != nil && p.DeferredReceiver != nil {
		return fmt.Errorf("provider %q: deferred_transport and deferred_receiver cannot be combined", p.Name)
	}
	if p.ForwardAuthorization {
		return fmt.Errorf("provider %q: forward_authorization cannot be combined with deferred_transport or deferred_receiver", p.Name)
	}
	if d := p.DeferredTransport; d != nil {
		if p.Type != ProviderTypeOpenAICompatible {
			return fmt.Errorf("provider %q: deferred_transport is only supported by static openai-compatible providers", p.Name)
		}
		if p.APIKey == "" && (p.APIKeyRef == nil || p.APIKeyRef.Key == "") { // pragma: allowlist secret
			return fmt.Errorf("provider %q: deferred_transport requires a configured api_key or api_key_ref, including on disabled providers", p.Name)
		}
		for _, field := range []struct {
			name string
			val  time.Duration
			min  time.Duration
			max  time.Duration
		}{
			{"control_timeout", d.ControlTimeout, 100 * time.Millisecond, 30 * time.Second},
			{"max_wait", d.MaxWait, time.Second, 2 * time.Hour},
			{"poll_interval", d.PollInterval, 50 * time.Millisecond, 5 * time.Second},
		} {
			if field.val < field.min || field.val > field.max {
				return fmt.Errorf("provider %q: deferred_transport.%s must be between %s and %s", p.Name, field.name, field.min, field.max)
			}
		}
		if d.MaxControlCalls < 4 || d.MaxControlCalls > 65536 {
			return fmt.Errorf("provider %q: deferred_transport.max_control_calls must be between 4 and 65536", p.Name)
		}
		if d.MaxControlRetries < 0 || d.MaxControlRetries > 32 {
			return fmt.Errorf("provider %q: deferred_transport.max_control_retries must be between 0 and 32", p.Name)
		}
		if err := validateDeferredBaseURL(p); err != nil {
			return err
		}
	}
	if d := p.DeferredReceiver; d != nil {
		if !root.Enabled {
			return fmt.Errorf("provider %q: deferred_receiver requires a root deferred block", p.Name)
		}
		if d.ExecutionTimeout < time.Second || d.ExecutionTimeout > root.ExecutionTimeout {
			return fmt.Errorf("provider %q: deferred_receiver.execution_timeout must be between 1s and the root execution_timeout", p.Name)
		}
		if d.MaxResultBytes < 1<<10 || d.MaxResultBytes > root.MaxResultBytes {
			return fmt.Errorf("provider %q: deferred_receiver.max_result_bytes must be between 1024 and the root max_result_bytes", p.Name)
		}
	}
	for _, model := range p.Models {
		if HasCapability(EffectiveCapabilities(p.Type, model), CapabilityChat) {
			return nil
		}
	}
	return fmt.Errorf("provider %q: deferred opt-ins require at least one chat-capable model", p.Name)
}

func ValidateManagedProviderDeferred(p Provider) error {
	if p.DeferredTransport != nil || p.DeferredReceiver != nil {
		return fmt.Errorf("provider %q: deferred_transport and deferred_receiver are static-config-only; managed providers cannot declare or inherit deferred opt-ins", p.Name)
	}
	return nil
}

func IsDeferredHeader(name string) bool {
	lower := strings.ToLower(name)
	return lower == "x-aiproxy-deferred" || strings.HasPrefix(lower, "x-aiproxy-deferred-")
}

func validateDeferredBaseURL(p Provider) error {
	if err := validateProviderBaseURL(p); err != nil {
		return err
	}
	u, err := url.Parse(p.BaseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("provider %q: deferred_transport requires an absolute base_url", p.Name)
	}
	if u.RawQuery != "" || u.ForceQuery || strings.Contains(p.BaseURL, "#") {
		return fmt.Errorf("provider %q: deferred_transport base_url must not include a query or fragment", p.Name)
	}
	path := u.EscapedPath()
	for {
		lower := strings.ToLower(path)
		if strings.Contains(path, `\`) || strings.Contains(lower, "%2f") || strings.Contains(lower, "%5c") {
			return fmt.Errorf("provider %q: deferred_transport base_url must not include backslashes or encoded path separators", p.Name)
		}
		for _, segment := range strings.Split(path, "/") {
			if segment == "." || segment == ".." {
				return fmt.Errorf("provider %q: deferred_transport base_url must not include dot segments", p.Name)
			}
		}
		next, err := url.PathUnescape(path)
		if err != nil || next == path {
			return nil
		}
		path = next
	}
}

func deferredDeclarationBlocks(body hcl.Body, owner string, names ...string) ([]*hcl.Block, error) {
	attrSchema := &hcl.BodySchema{}
	blockSchema := &hcl.BodySchema{}
	for _, name := range names {
		attrSchema.Attributes = append(attrSchema.Attributes, hcl.AttributeSchema{Name: name})
		blockSchema.Blocks = append(blockSchema.Blocks, hcl.BlockHeaderSchema{Type: name})
	}
	attrs, _, diags := body.PartialContent(attrSchema)
	if diags.HasErrors() {
		return nil, fmt.Errorf("%s: cannot inspect deferred declarations", owner)
	}
	_, native := body.(*hclsyntax.Body)
	for name, attr := range attrs.Attributes {
		if native {
			return nil, fmt.Errorf("%s: %s must be an unlabeled block", owner, name)
		}
		value, diags := attr.Expr.Value(nil)
		if diags.HasErrors() || !value.IsKnown() || value.IsNull() {
			return nil, fmt.Errorf("%s: %s must be a non-null block object", owner, name)
		}
		if value.Type().IsTupleType() || value.Type().IsListType() {
			if value.LengthInt() != 1 {
				return nil, fmt.Errorf("%s: exactly one %s block object is required when declared", owner, name)
			}
			it := value.ElementIterator()
			it.Next()
			_, value = it.Element()
		}
		if !value.IsKnown() || value.IsNull() || !value.Type().IsObjectType() {
			return nil, fmt.Errorf("%s: %s must be a non-null block object", owner, name)
		}
	}
	content, _, diags := body.PartialContent(blockSchema)
	if diags.HasErrors() {
		return nil, fmt.Errorf("%s: deferred settings require unlabeled block objects", owner)
	}
	seen := make(map[string]bool)
	for _, block := range content.Blocks {
		if seen[block.Type] {
			return nil, fmt.Errorf("%s: only one %s block is supported", owner, block.Type)
		}
		seen[block.Type] = true
		attrs, diags := block.Body.JustAttributes()
		if diags.HasErrors() {
			return nil, fmt.Errorf("%s: %s supports attributes only", owner, block.Type)
		}
		for name, attr := range attrs {
			value, diags := attr.Expr.Value(nil)
			if diags.HasErrors() || !value.IsKnown() || value.IsNull() {
				return nil, fmt.Errorf("%s: %s.%s must not be null or unknown", owner, block.Type, name)
			}
		}
	}
	return content.Blocks, nil
}

func validateDeferredDeclarations(body hcl.Body) (bool, error) {
	root, err := deferredDeclarationBlocks(body, "root", "deferred")
	if err != nil {
		return false, err
	}
	found := len(root) != 0
	content, _, diags := body.PartialContent(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{{Type: "provider", LabelNames: []string{"type", "name"}}},
	})
	if diags.HasErrors() {
		return false, fmt.Errorf("cannot inspect deferred provider declarations")
	}
	for _, provider := range content.Blocks {
		blocks, err := deferredDeclarationBlocks(provider.Body, fmt.Sprintf("provider %q", provider.Labels[1]), "deferred_transport", "deferred_receiver")
		if err != nil {
			return false, err
		}
		found = found || len(blocks) != 0
	}
	return found, nil
}

func ValidateDeferredConfig(source []byte, filename string) error {
	file, err := parseConfig(expandEnvCalls(trimLeadingWhitespace(source)), filename)
	if err != nil {
		return err
	}
	return validateDeferredConfigBody(file.Body)
}

func validateDeferredConfigBody(body hcl.Body) error {
	found, err := validateDeferredDeclarations(body)
	if err != nil || !found {
		return err
	}
	var raw struct {
		Deferred  []*rawDeferred `hcl:"deferred,block"`
		Auth      []rawAuth      `hcl:"auth,block"`
		Providers []rawProvider  `hcl:"provider,block"`
		Remain    hcl.Body       `hcl:",remain"`
	}
	if diags := gohcl.DecodeBody(body, nil, &raw); diags.HasErrors() {
		return fmt.Errorf("decode deferred config: %s", diags.Error())
	}
	root := DefaultDeferred()
	if len(raw.Deferred) > 0 {
		root, err = buildDeferred(raw.Deferred[0])
		if err != nil {
			return err
		}
		if len(raw.Auth) != 1 || AuthMode(raw.Auth[0].Mode) != AuthModeBearerStatic {
			return fmt.Errorf("deferred requires auth mode bearer_static")
		}
	}
	byName := make(map[string]rawProvider, len(raw.Providers))
	for _, p := range raw.Providers {
		byName[p.Name] = p
	}
	for _, local := range raw.Providers {
		if local.DeferredTransport == nil && local.DeferredReceiver == nil {
			continue
		}
		effective := local
		if local.Extends != "" {
			base, ok := byName[local.Extends]
			if !ok || base.Extends != "" || base.Type != local.Type {
				return fmt.Errorf("provider %q: deferred opt-in requires a concrete same-type base provider", local.Name)
			}
			effective.BaseURL, effective.Models = base.BaseURL, base.Models
			effective.AllowInsecureBaseURL = base.AllowInsecureBaseURL
		}
		p := Provider{
			Type: ProviderType(local.Type), Name: local.Name, BaseURL: effective.BaseURL,
			APIKey: local.APIKey, ForwardAuthorization: local.ForwardAuthorization,
			AllowInsecureBaseURL: effective.AllowInsecureBaseURL != nil && *effective.AllowInsecureBaseURL,
		}
		if local.APIKeyRef != nil { // pragma: allowlist secret
			p.APIKeyRef = &APIKeyRef{Path: local.APIKeyRef.Path, Key: local.APIKeyRef.Key}
		}
		for _, m := range effective.Models {
			model := Model{Name: m.Name, Protocol: ModelProtocol(m.Protocol)}
			for _, capability := range m.Capabilities {
				model.Capabilities = append(model.Capabilities, Capability(capability))
			}
			p.Models = append(p.Models, model)
		}
		if err := buildProviderDeferred(&p, local, root); err != nil {
			return fmt.Errorf("provider %q: %w", p.Name, err)
		}
	}
	return nil
}
