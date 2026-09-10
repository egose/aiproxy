package provider

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type CooldownError struct {
	Err   error
	Delay time.Duration
	OK    bool
}

func (e *CooldownError) Error() string {
	if e == nil || e.Err == nil {
		return "upstream cooldown"
	}
	return e.Err.Error()
}

func (e *CooldownError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func CooldownDelayFromError(err error) (time.Duration, bool) {
	var cooldown *CooldownError
	if errors.As(err, &cooldown) && cooldown != nil && cooldown.OK && cooldown.Delay > 0 {
		return cooldown.Delay, true
	}
	return 0, false
}

func withCooldownError(err error, delay time.Duration, ok bool) error {
	if err == nil || !ok || delay <= 0 {
		return err
	}
	return &CooldownError{Err: err, Delay: delay, OK: true}
}

func attachCooldownDelay(res *Result, delay time.Duration, ok bool) *Result {
	if res == nil || !ok || delay <= 0 {
		return res
	}
	res.RetryDelay = delay
	res.HasRetryDelay = true
	return res
}

const defaultQuotaCooldown = 60 * time.Second

func ParseRetryCooldown(header http.Header, now time.Time) (time.Duration, bool) {
	if delay, ok := parseRetryAfterMS(header); ok {
		return delay, true
	}
	if delay, ok := parseRetryAfter(header, now); ok {
		return delay, true
	}
	return parseQuotaExhausted(header)
}

func parseQuotaExhausted(header http.Header) (time.Duration, bool) {
	for _, name := range []string{"x-ratelimit-remaining-tokens", "x-ratelimit-remaining-requests"} {
		for _, value := range headerValuesFold(header, name) {
			trimmed := strings.Trim(value, " \t")
			if trimmed == "" || !isASCIIDigits(trimmed) {
				continue
			}
			remaining, err := strconv.ParseUint(trimmed, 10, 64)
			if err != nil {
				continue
			}
			if remaining == 0 {
				return defaultQuotaCooldown, true
			}
		}
	}
	return 0, false
}

func headerValuesFold(header http.Header, name string) []string {
	var out []string
	for key, values := range header {
		if strings.EqualFold(key, name) {
			out = append(out, values...)
		}
	}
	return out
}

func parseRetryAfterMS(header http.Header) (time.Duration, bool) {
	for _, value := range headerValuesFold(header, "retry-after-ms") {
		delay, ok := parseCooldownMillis(strings.Trim(value, " \t"))
		if !ok {
			continue
		}
		if delay <= 0 {
			return 0, false
		}
		return delay, true
	}
	return 0, false
}

func parseCooldownMillis(trimmed string) (time.Duration, bool) {
	if trimmed == "" || !isASCIIDigits(trimmed) {
		return 0, false
	}
	millis, err := strconv.ParseUint(trimmed, 10, 64)
	if err != nil {
		return 0, false
	}
	const maxMillis = uint64(^uint64(0)>>1) / uint64(time.Millisecond)
	if millis > maxMillis {
		return 0, false
	}
	return time.Duration(millis) * time.Millisecond, true
}

func parseRetryAfter(header http.Header, now time.Time) (time.Duration, bool) {
	for _, value := range headerValuesFold(header, "Retry-After") {
		token := strings.Trim(strings.SplitN(value, ",", 2)[0], " \t")
		if token != "" && isASCIIDigits(token) {
			delay, ok := parseCooldownSeconds(token)
			if !ok {
				continue
			}
			if delay <= 0 {
				return 0, false
			}
			return delay, true
		}
		when, err := http.ParseTime(strings.Trim(value, " \t"))
		if err != nil {
			continue
		}
		delay := when.Sub(now)
		if delay <= 0 {
			return 0, false
		}
		if delay == time.Duration(1<<63-1) {
			return 0, false
		}
		return delay, true
	}
	return 0, false
}

func parseCooldownSeconds(trimmed string) (time.Duration, bool) {
	seconds, err := strconv.ParseUint(trimmed, 10, 64)
	if err != nil {
		return 0, false
	}
	const maxSeconds = uint64(^uint64(0)>>1) / uint64(time.Second)
	if seconds > maxSeconds {
		return 0, false
	}
	return time.Duration(seconds) * time.Second, true
}

func isASCIIDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func ceilMillis(d time.Duration) int64 {
	ms := int64(d / time.Millisecond)
	if d%time.Millisecond != 0 {
		ms++
	}
	if ms < 1 {
		ms = 1
	}
	return ms
}

func ceilSeconds(d time.Duration) int64 {
	sec := int64(d / time.Second)
	if d%time.Second != 0 {
		sec++
	}
	if sec < 1 {
		sec = 1
	}
	return sec
}

func SyntheticCooldownResult(remaining time.Duration) *Result {
	if remaining <= 0 {
		remaining = time.Millisecond
	}
	millis := ceilMillis(remaining)
	seconds := ceilSeconds(remaining)
	header := http.Header{
		"Content-Type":   []string{"application/json"},
		"Retry-After":    []string{strconv.FormatInt(seconds, 10)},
		"Retry-After-Ms": []string{strconv.FormatInt(millis, 10)},
	}
	body := []byte(`{"error":{"type":"upstream_rate_limited","message":"all alias targets cooling, retry after ` + strconv.FormatInt(millis, 10) + `ms"}}`)
	return &Result{StatusCode: http.StatusTooManyRequests, Header: header, Body: body}
}
