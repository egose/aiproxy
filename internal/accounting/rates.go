package accounting

import "time"

const rateSeconds = 15 * 60

type RateCounts struct {
	Requests  int64 `json:"requests"`
	Errors    int64 `json:"errors"`
	Throttled int64 `json:"throttled"`
	Tokens    int64 `json:"tokens"`
}

type RateSnapshot struct {
	WindowEnd     time.Time      `json:"window_end"`
	BucketSeconds int            `json:"bucket_seconds"`
	Minute        RateCounts     `json:"minute"`
	FiveMinutes   RateCounts     `json:"five_minutes"`
	Minutes       [15]RateCounts `json:"minutes"`
}

type rateBucket struct {
	start  time.Time
	counts RateCounts
}

func (c *RateCounts) add(other RateCounts) {
	c.Requests += other.Requests
	c.Errors += other.Errors
	c.Throttled += other.Throttled
	c.Tokens += other.Tokens
}

func (a *Aggregator) recordRateLocked(event Event, timestamp, now time.Time) {
	start := timestamp.Truncate(time.Second)
	if start.Before(now.Truncate(time.Second).Add(-rateSeconds * time.Second)) {
		return
	}
	i := start.Unix() % int64(len(a.rates))
	if i < 0 {
		i += int64(len(a.rates))
	}
	b := &a.rates[i]
	if !b.start.Equal(start) {
		*b = rateBucket{start: start}
	}
	b.counts.Requests++
	if isErrorStatus(event.StatusCode) {
		b.counts.Errors++
	}
	if event.StatusCode == 429 {
		b.counts.Throttled++
	}
	b.counts.Tokens += event.TotalTokens
}

func (a *Aggregator) RateSnapshot() *RateSnapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	end := a.nowTime(time.Time{}).Truncate(time.Second)
	start := end.Add(-rateSeconds * time.Second)
	out := &RateSnapshot{WindowEnd: end, BucketSeconds: 1}
	for i := range a.rates {
		b := &a.rates[i]
		if b.start.Before(start) {
			*b = rateBucket{}
			continue
		}
		if !b.start.Before(end) {
			continue
		}
		minute := int(b.start.Sub(start) / time.Minute)
		out.Minutes[minute].add(b.counts)
		if minute >= 10 {
			out.FiveMinutes.add(b.counts)
		}
		if minute == 14 {
			out.Minute.add(b.counts)
		}
	}
	return out
}
