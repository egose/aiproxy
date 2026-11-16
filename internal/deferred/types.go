package deferred

import (
	"net/http"
	"time"
)

const (
	Protocol            = "aiproxy-deferred"
	Version             = 1
	Prefix              = "/_internal/deferred/v1"
	MediaType           = "application/vnd.aiproxy.deferred+json"
	SubmitWindow        = 30 * time.Second
	MaxReadBytes        = 64 << 10
	MaxControlBodyBytes = 128 << 10
	MaxMetadataBytes    = 16 << 10
	MaxErrorBytes       = 1 << 10
	MaxSubmitTokenBytes = 512
	ControlScratchBytes = 512 << 10
	ResultSlabBytes     = 32 << 10
)

const (
	HardRequestBytes    int64 = 8 << 20
	JobMetadataBytes    int64 = 64 << 10
	DedupEntryBytes     int64 = 4 << 10
	OwnerBucketBytes    int64 = 1 << 10
	PollAfterMS         int64 = 250
	JSONDepth                 = 64
	JSONTokens                = 16384
	ControlJSONDepth          = 8
	ControlJSONTokens         = 2048
	SSELineBytes              = 256 << 10
	SSEEventBytes             = 1 << 20
	SSEDataLines              = 1024
	FinalizationWorkers       = 2
)

type State string
type OutcomeKind string
type Acceptance string

const (
	StateRunning      State = "running"
	StateHeadersReady State = "headers_ready"
	StateCompleted    State = "completed"
	StateFailed       State = "failed"
	StateCancelled    State = "cancelled"

	OutcomeResponse         OutcomeKind = "response"
	OutcomeUpstreamError    OutcomeKind = "upstream_error"
	OutcomeInvalidRequest   OutcomeKind = "invalid_request"
	OutcomeExecutionTimeout OutcomeKind = "execution_timeout"
	OutcomeLimitExceeded    OutcomeKind = "limit_exceeded"
	OutcomeCancelled        OutcomeKind = "cancelled"

	AcceptanceNotAccepted   Acceptance = "not_accepted"
	AcceptanceAccepted      Acceptance = "accepted"
	AcceptanceIndeterminate Acceptance = "indeterminate"
)

type Envelope struct {
	Protocol string `json:"protocol"`
	Version  int    `json:"version"`
}

type Limits struct {
	RequestBytes          int64 `json:"request_bytes"`
	ResultBytes           int64 `json:"result_bytes"`
	ExecutionTimeoutMS    int64 `json:"execution_timeout_ms"`
	RetentionMS           int64 `json:"retention_ms"`
	ControlTimeoutMS      int64 `json:"control_timeout_ms"`
	FinalizationTimeoutMS int64 `json:"finalization_timeout_ms"`
	ReadBytes             int   `json:"read_bytes"`
}

type Capability struct {
	Envelope
	Generation    string `json:"generation"`
	PublicModel   string `json:"public_model"`
	SubmitToken   string `json:"submit_token"`
	SubmitUntilMS int64  `json:"submit_until_ms"`
	Limits        Limits `json:"limits"`
	PollAfterMS   int64  `json:"poll_after_ms"`
}

type SubmitRequest struct {
	Envelope
	Generation  string      `json:"generation"`
	SubmitToken string      `json:"submit_token"`
	AttemptKey  string      `json:"attempt_key"`
	Headers     http.Header `json:"headers"`
	Body        []byte      `json:"body"`
}

type Receipt struct {
	Envelope
	Generation          string `json:"generation"`
	ID                  string `json:"id"`
	PublicModel         string `json:"public_model"`
	AttemptKey          string `json:"attempt_key"`
	ScopeDigest         string `json:"scope_digest"`
	Fingerprint         string `json:"fingerprint"`
	State               State  `json:"state"`
	ExecutionDeadlineMS int64  `json:"execution_deadline_ms"`
	DedupUntilMS        int64  `json:"dedup_until_ms"`
	PollAfterMS         int64  `json:"poll_after_ms"`
	Replayed            bool   `json:"replayed"`
}

type RetryAdvice struct {
	ObservedAtMS int64 `json:"observed_at_ms"`
	DeadlineMS   int64 `json:"deadline_ms"`
	RemainingMS  int64 `json:"remaining_ms"`
}

type ResponseHead struct {
	StatusCode int          `json:"status_code"`
	Headers    http.Header  `json:"headers"`
	Streaming  bool         `json:"streaming"`
	Retry      *RetryAdvice `json:"retry,omitempty"`
}

type Usage struct {
	PromptTokens        int64 `json:"prompt_tokens"`
	CompletionTokens    int64 `json:"completion_tokens"`
	TotalTokens         int64 `json:"total_tokens"`
	CachedTokens        int64 `json:"cached_tokens"`
	CacheCreationTokens int64 `json:"cache_creation_tokens"`
	CacheReadTokens     int64 `json:"cache_read_tokens"`
}

type ErrorDetail struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	BlockID string `json:"block_id,omitempty"`
}

type Outcome struct {
	Kind  OutcomeKind  `json:"kind"`
	Usage Usage        `json:"usage"`
	Error *ErrorDetail `json:"error,omitempty"`
}

type JobInfo struct {
	Receipt
	Response       *ResponseHead `json:"response,omitempty"`
	Outcome        *Outcome      `json:"outcome,omitempty"`
	AvailableBytes uint64        `json:"available_bytes,string"`
	RetainUntilMS  int64         `json:"retain_until_ms,omitempty"`
}

type ResultManifest struct {
	JobInfo
	BodySHA256 string `json:"body_sha256"`
}

type ReadFrame struct {
	JobInfo
	Cursor     uint64 `json:"cursor,string"`
	NextCursor uint64 `json:"next_cursor,string"`
	Data       []byte `json:"data"`
	Done       bool   `json:"done"`
}

type ControlError struct {
	Envelope
	Error       ErrorDetail `json:"error"`
	Acceptance  Acceptance  `json:"acceptance"`
	PollAfterMS int64       `json:"poll_after_ms,omitempty"`
}

type Owner struct {
	Kind      string
	ID        string
	Tenant    string
	Workspace string
}

type ReceiverLimits struct {
	ExecutionTimeout time.Duration
	MaxResultBytes   int64
}

type ProtocolError struct {
	Status     int
	Wire       ControlError
	Admission  bool
	RetryAfter string
}

func (e *ProtocolError) Error() string {
	return e.Wire.Error.Type + ": " + e.Wire.Error.Message
}

func NewProtocolError(status int, detail ErrorDetail, acceptance Acceptance) *ProtocolError {
	if err := ValidateErrorDetail(detail); err != nil {
		detail = ErrorDetail{Type: "invalid_request", Message: "invalid error metadata"}
	}
	return &ProtocolError{Status: status, Wire: ControlError{Envelope: NewEnvelope(), Error: detail, Acceptance: acceptance}}
}

func protocolError(status int, code, message string, acceptance Acceptance) *ProtocolError {
	return NewProtocolError(status, ErrorDetail{Type: code, Message: message}, acceptance)
}

func NewEnvelope() Envelope {
	return Envelope{Protocol: Protocol, Version: Version}
}

func (s State) Terminal() bool {
	return s == StateCompleted || s == StateFailed || s == StateCancelled
}
