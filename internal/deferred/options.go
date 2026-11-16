package deferred

import (
	"fmt"
	"math"
	"time"

	"github.com/egose/aiproxy/internal/config"
)

type Options struct {
	Enabled             bool
	MaxJobs             int
	MaxRunning          int
	MaxAdmissions       int
	MaxEntries          int
	MaxMemoryBytes      int64
	MaxRequestBytes     int64
	MaxResultBytes      int64
	MaxWorkBytes        int64
	PerOwnerJobs        int
	PerOwnerRunning     int
	MaxControlInflight  int
	ExecutionTimeout    time.Duration
	Retention           time.Duration
	ControlTimeout      time.Duration
	FinalizationTimeout time.Duration
}

func OptionsFromConfig(d config.Deferred) Options {
	return Options{
		Enabled: d.Enabled, MaxJobs: d.MaxJobs, MaxRunning: d.MaxRunning,
		MaxAdmissions: d.MaxAdmissions, MaxEntries: d.MaxEntries,
		MaxMemoryBytes: d.MaxMemoryBytes, MaxRequestBytes: d.MaxRequestBytes,
		MaxResultBytes: d.MaxResultBytes, MaxWorkBytes: d.MaxWorkBytes,
		PerOwnerJobs: d.PerOwnerJobs, PerOwnerRunning: d.PerOwnerRunning,
		MaxControlInflight: d.MaxControlInflight, ExecutionTimeout: d.ExecutionTimeout,
		Retention: d.Retention, ControlTimeout: d.ControlTimeout,
		FinalizationTimeout: d.FinalizationTimeout,
	}
}

func DefaultOptions() Options {
	return OptionsFromConfig(config.DefaultDeferred())
}

func (o Options) Validate() error {
	return config.ValidateDeferred(config.Deferred{
		Enabled: o.Enabled, MaxJobs: o.MaxJobs, MaxRunning: o.MaxRunning,
		MaxAdmissions: o.MaxAdmissions, MaxEntries: o.MaxEntries,
		MaxMemoryBytes: o.MaxMemoryBytes, MaxRequestBytes: o.MaxRequestBytes,
		MaxResultBytes: o.MaxResultBytes, MaxWorkBytes: o.MaxWorkBytes,
		PerOwnerJobs: o.PerOwnerJobs, PerOwnerRunning: o.PerOwnerRunning,
		MaxControlInflight: o.MaxControlInflight, ExecutionTimeout: o.ExecutionTimeout,
		Retention: o.Retention, ControlTimeout: o.ControlTimeout,
		FinalizationTimeout: o.FinalizationTimeout,
	})
}

func (o Options) FixedMemoryBytes() (int64, error) {
	scratch, err := SubmitScratchBytes(o.MaxRequestBytes)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, allowance := range []struct {
		count int
		bytes int64
	}{
		{o.MaxControlInflight, ControlScratchBytes},
		{o.MaxJobs, JobMetadataBytes},
		{o.MaxEntries, DedupEntryBytes},
		{o.MaxEntries, OwnerBucketBytes},
		{o.MaxAdmissions, scratch},
	} {
		if allowance.count < 0 || allowance.bytes < 0 || (allowance.count > 0 && allowance.bytes > (math.MaxInt64-total)/int64(allowance.count)) {
			return 0, fmt.Errorf("deferred memory allowances overflow int64")
		}
		total += int64(allowance.count) * allowance.bytes
	}
	return total, nil
}

func SubmitWireBytes(requestBytes int64) (int64, error) {
	if requestBytes < 0 || requestBytes > HardRequestBytes {
		return 0, fmt.Errorf("invalid deferred request size")
	}
	return (requestBytes+2)/3*4 + MaxMetadataBytes, nil
}

func SubmitScratchBytes(requestBytes int64) (int64, error) {
	wire, err := SubmitWireBytes(requestBytes)
	if err != nil {
		return 0, err
	}
	return wire + requestBytes, nil
}

func RoundedResultBytes(n int64) (int64, error) {
	if n < 0 || n > math.MaxInt64-(ResultSlabBytes-1) {
		return 0, fmt.Errorf("deferred result capacity overflow")
	}
	return (n + ResultSlabBytes - 1) / ResultSlabBytes * ResultSlabBytes, nil
}

func (o Options) limits(receiver ReceiverLimits) (Limits, error) {
	execution, result := o.ExecutionTimeout, o.MaxResultBytes
	if receiver.ExecutionTimeout != 0 {
		if receiver.ExecutionTimeout < time.Second || receiver.ExecutionTimeout > execution {
			return Limits{}, fmt.Errorf("receiver execution timeout exceeds root policy")
		}
		execution = receiver.ExecutionTimeout
	}
	if receiver.MaxResultBytes != 0 {
		if receiver.MaxResultBytes < 1<<10 || receiver.MaxResultBytes > result {
			return Limits{}, fmt.Errorf("receiver result allowance exceeds root policy")
		}
		result = receiver.MaxResultBytes
	}
	return Limits{
		RequestBytes: o.MaxRequestBytes, ResultBytes: result,
		ExecutionTimeoutMS: execution.Milliseconds(), RetentionMS: o.Retention.Milliseconds(),
		ControlTimeoutMS: o.ControlTimeout.Milliseconds(), FinalizationTimeoutMS: o.FinalizationTimeout.Milliseconds(),
		ReadBytes: MaxReadBytes,
	}, nil
}
