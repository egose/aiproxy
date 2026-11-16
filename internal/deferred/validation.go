package deferred

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
)

func ValidateID(id string) error {
	if len(id) != 22 {
		return fmt.Errorf("invalid deferred identifier")
	}
	var raw [16]byte
	n, err := base64.RawURLEncoding.Strict().Decode(raw[:], []byte(id))
	if err != nil || n != len(raw) || base64.RawURLEncoding.EncodeToString(raw[:]) != id {
		return fmt.Errorf("invalid deferred identifier")
	}
	return nil
}

func ValidateAttemptKey(key string) error {
	if len(key) < 22 || len(key) > 64 {
		return fmt.Errorf("invalid deferred attempt key")
	}
	for i := 0; i < len(key); i++ {
		c := key[i]
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return fmt.Errorf("invalid deferred attempt key")
		}
	}
	return nil
}

func ValidatePublicModel(model string) error {
	if model == "" || len(model) > 512 || !utf8.ValidString(model) {
		return fmt.Errorf("invalid deferred public model")
	}
	for _, c := range model {
		if c <= ' ' || c == 0x7f {
			return fmt.Errorf("invalid deferred public model")
		}
	}
	return nil
}

func ValidateOwner(owner Owner) error {
	if owner.Kind != "static" && owner.Kind != "key" {
		return fmt.Errorf("deferred ownership requires an inference identity")
	}
	if owner.ID == "" || owner.Kind == "static" && owner.Workspace != "" || owner.Kind == "key" && (owner.Workspace == "" || owner.Tenant != "") {
		return fmt.Errorf("invalid deferred owner")
	}
	for _, value := range []string{owner.ID, owner.Tenant, owner.Workspace} {
		if len(value) > 256 || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
			return fmt.Errorf("invalid deferred owner")
		}
	}
	return nil
}

func ParseCursor(value string) (uint64, error) {
	if value == "" || len(value) > 20 || len(value) > 1 && value[0] == '0' {
		return 0, fmt.Errorf("invalid deferred cursor")
	}
	for i := range value {
		if value[i] < '0' || value[i] > '9' {
			return 0, fmt.Errorf("invalid deferred cursor")
		}
	}
	n, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid deferred cursor")
	}
	return n, nil
}

func ParseReadLimit(value string) (int, error) {
	if value == "" {
		return MaxReadBytes, nil
	}
	n, err := ParseCursor(value)
	if err != nil || n == 0 || n > MaxReadBytes {
		return 0, fmt.Errorf("invalid deferred read limit")
	}
	return int(n), nil
}

func ValidateLimits(l Limits) error {
	if l.RequestBytes < 1<<10 || l.RequestBytes > HardRequestBytes || l.ResultBytes < 1<<10 || l.ResultBytes > 32<<20 || l.ReadBytes != MaxReadBytes {
		return fmt.Errorf("invalid deferred byte limits")
	}
	for _, bound := range []struct {
		value int64
		min   int64
		max   int64
	}{
		{l.ExecutionTimeoutMS, 1000, 3600000},
		{l.RetentionMS, 10000, 3600000},
		{l.ControlTimeoutMS, 100, 30000},
		{l.FinalizationTimeoutMS, 100, 30000},
	} {
		if bound.value < bound.min || bound.value > bound.max {
			return fmt.Errorf("invalid deferred duration limits")
		}
	}
	return nil
}

func validateCapability(capability Capability) error {
	if capability.Envelope != NewEnvelope() || ValidateID(capability.Generation) != nil || ValidatePublicModel(capability.PublicModel) != nil ||
		capability.SubmitToken == "" || len(capability.SubmitToken) > MaxSubmitTokenBytes || capability.SubmitUntilMS <= 0 ||
		ValidateLimits(capability.Limits) != nil || capability.PollAfterMS < 50 || capability.PollAfterMS > 2000 {
		return fmt.Errorf("invalid deferred capability")
	}
	return nil
}

func ValidateErrorDetail(detail ErrorDetail) error {
	if detail.Type == "" || len(detail.Type)+len(detail.Message)+len(detail.BlockID) > MaxErrorBytes {
		return fmt.Errorf("invalid deferred error metadata")
	}
	for _, field := range []string{detail.Type, detail.Message, detail.BlockID} {
		if !utf8.ValidString(field) {
			return fmt.Errorf("invalid deferred error metadata")
		}
	}
	return nil
}

func validateOutcome(out Outcome, hasHead bool) error {
	for _, value := range []int64{
		out.Usage.PromptTokens, out.Usage.CompletionTokens, out.Usage.TotalTokens,
		out.Usage.CachedTokens, out.Usage.CacheCreationTokens, out.Usage.CacheReadTokens,
	} {
		if value < 0 {
			return fmt.Errorf("negative deferred usage")
		}
	}
	if out.Error != nil {
		if err := ValidateErrorDetail(*out.Error); err != nil {
			return err
		}
	}
	switch out.Kind {
	case OutcomeResponse:
		if !hasHead || out.Error != nil {
			return fmt.Errorf("deferred response requires a response head")
		}
	case OutcomeUpstreamError, OutcomeInvalidRequest, OutcomeExecutionTimeout, OutcomeLimitExceeded, OutcomeCancelled:
	default:
		return fmt.Errorf("invalid deferred outcome")
	}
	return nil
}

func validHeaderName(name string) bool {
	if name == "" || len(name) > MaxMetadataBytes {
		return false
	}
	for i := range name {
		c := name[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
			continue
		}
		if !strings.ContainsRune("!#$%&'*+-.^_`|~", rune(c)) {
			return false
		}
	}
	return true
}

func privateHeader(name string) bool {
	return name == "x-aiproxy-deferred" || strings.HasPrefix(name, "x-aiproxy-deferred-")
}

func hopHeader(name string) bool {
	switch name {
	case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade", "content-length":
		return true
	}
	return false
}

func credentialHeader(name string) bool {
	switch name {
	case "authorization", "cookie", "set-cookie", "x-api-key", "x-goog-api-key", "host":
		return true
	}
	return false
}

func validateHeaders(headers http.Header) error {
	if len(headers) > 64 {
		return fmt.Errorf("too many deferred header names")
	}
	names := make(map[string]bool, len(headers))
	values, bytes := 0, 0
	for name, list := range headers {
		if !validHeaderName(name) {
			return fmt.Errorf("invalid deferred header name")
		}
		lower := strings.ToLower(name)
		if names[lower] {
			return fmt.Errorf("duplicate deferred header name")
		}
		names[lower] = true
		values += len(list)
		bytes += len(name)
		if values > 128 || bytes > MaxMetadataBytes {
			return fmt.Errorf("deferred header metadata exceeds limit")
		}
		for _, value := range list {
			if len(value) > 4096 || !utf8.ValidString(value) {
				return fmt.Errorf("invalid deferred header value")
			}
			for i := range value {
				if value[i] < ' ' && value[i] != '\t' || value[i] == 0x7f {
					return fmt.Errorf("invalid deferred header value")
				}
			}
			bytes += len(value)
			if bytes > MaxMetadataBytes {
				return fmt.Errorf("deferred header metadata exceeds limit")
			}
		}
	}
	if _, err := JSONEncodedSize(headers, MaxMetadataBytes, ControlJSONDepth, ControlJSONTokens); err != nil {
		return fmt.Errorf("deferred header metadata exceeds limit")
	}
	return nil
}

func ValidateHeaderBag(headers http.Header) error {
	return validateHeaders(headers)
}

func CanonicalSubmitHeaders(headers http.Header) (http.Header, error) {
	if err := validateHeaders(headers); err != nil {
		return nil, err
	}
	out := make(http.Header, len(headers))
	for name, values := range headers {
		name = strings.ToLower(name)
		if privateHeader(name) || hopHeader(name) || credentialHeader(name) {
			return nil, fmt.Errorf("forbidden deferred submit header")
		}
		out[http.CanonicalHeaderKey(name)] = cloneStrings(values)
	}
	if values := out.Values("Content-Type"); len(values) != 1 || values[0] != "application/json" {
		return nil, fmt.Errorf("deferred inference content type must be application/json")
	}
	return out, nil
}

func FilterResponseHeaders(headers http.Header) (http.Header, error) {
	if err := validateHeaders(headers); err != nil {
		return nil, err
	}
	var connection []string
	for name, values := range headers {
		if strings.EqualFold(name, "Connection") {
			connection = values
			break
		}
	}
	out := make(http.Header, len(headers))
	for name, values := range headers {
		lower := strings.ToLower(name)
		if hopHeader(lower) || privateHeader(lower) || connectionHeader(connection, name) {
			continue
		}
		out[http.CanonicalHeaderKey(name)] = cloneStrings(values)
	}
	return out, nil
}

func connectionHeader(values []string, name string) bool {
	for _, value := range values {
		for token := range strings.SplitSeq(value, ",") {
			if strings.EqualFold(strings.TrimSpace(token), name) {
				return true
			}
		}
	}
	return false
}

func cloneStrings(values []string) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = strings.Clone(value)
	}
	return out
}

func cloneOwner(owner Owner) Owner {
	return Owner{Kind: strings.Clone(owner.Kind), ID: strings.Clone(owner.ID), Tenant: strings.Clone(owner.Tenant), Workspace: strings.Clone(owner.Workspace)}
}

func validateHash(value string) error {
	if len(value) != 64 || value != strings.ToLower(value) {
		return fmt.Errorf("invalid deferred hash")
	}
	var decoded [32]byte
	if _, err := hex.Decode(decoded[:], []byte(value)); err != nil {
		return fmt.Errorf("invalid deferred hash")
	}
	return nil
}

func ValidateReceipt(receipt Receipt) error {
	if receipt.Envelope != NewEnvelope() || ValidateID(receipt.Generation) != nil || ValidateID(receipt.ID) != nil ||
		ValidatePublicModel(receipt.PublicModel) != nil || ValidateAttemptKey(receipt.AttemptKey) != nil ||
		validateHash(receipt.ScopeDigest) != nil || validateHash(receipt.Fingerprint) != nil ||
		receipt.ExecutionDeadlineMS <= 0 || receipt.DedupUntilMS < receipt.ExecutionDeadlineMS || receipt.PollAfterMS < 50 || receipt.PollAfterMS > 2000 {
		return fmt.Errorf("invalid deferred receipt")
	}
	switch receipt.State {
	case StateRunning, StateHeadersReady, StateCompleted, StateFailed, StateCancelled:
		return nil
	}
	return fmt.Errorf("invalid deferred state")
}

func ValidateJobInfo(info JobInfo) error {
	if err := ValidateReceipt(info.Receipt); err != nil {
		return err
	}
	if info.AvailableBytes > 32<<20 {
		return fmt.Errorf("invalid deferred available bytes")
	}
	if info.Response != nil {
		if info.Response.StatusCode < 200 || info.Response.StatusCode > 599 || validateHeaders(info.Response.Headers) != nil {
			return fmt.Errorf("invalid deferred response head")
		}
		if retry := info.Response.Retry; retry != nil && (retry.ObservedAtMS <= 0 || retry.DeadlineMS < retry.ObservedAtMS || retry.RemainingMS < 0 || retry.RemainingMS > retry.DeadlineMS-retry.ObservedAtMS) {
			return fmt.Errorf("invalid deferred retry advice")
		}
	}
	if info.State.Terminal() {
		if info.Outcome == nil || info.RetainUntilMS <= 0 || validateOutcome(*info.Outcome, info.Response != nil) != nil {
			return fmt.Errorf("invalid deferred terminal outcome")
		}
		if info.State == StateCompleted && info.Outcome.Kind != OutcomeResponse || info.State == StateCancelled && info.Outcome.Kind != OutcomeCancelled || info.State == StateFailed && (info.Outcome.Kind == OutcomeResponse || info.Outcome.Kind == OutcomeCancelled) {
			return fmt.Errorf("deferred state/outcome mismatch")
		}
	} else if info.Outcome != nil || info.RetainUntilMS != 0 || info.State == StateHeadersReady && info.Response == nil {
		return fmt.Errorf("invalid deferred active outcome")
	}
	return nil
}

func ValidateReadFrame(frame ReadFrame) error {
	if err := ValidateJobInfo(frame.JobInfo); err != nil {
		return err
	}
	if frame.Data == nil || len(frame.Data) > MaxReadBytes || frame.Cursor > frame.AvailableBytes ||
		frame.Cursor > math.MaxUint64-uint64(len(frame.Data)) || frame.NextCursor != frame.Cursor+uint64(len(frame.Data)) ||
		frame.NextCursor > frame.AvailableBytes || frame.Done != (frame.State.Terminal() && frame.NextCursor == frame.AvailableBytes) {
		return fmt.Errorf("invalid deferred read frame")
	}
	return nil
}
