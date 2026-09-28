package dashboard

import (
	"slices"
)

type pausedResults struct {
	payloadList   *payloadListMsg
	payloadDetail *payloadDetailMsg
	blockList     *blockListMsg
	blockDetail   *blockDetailMsg
	blockDecision *blockDecisionMsg
}

func (m *model) togglePause() {
	m.paused = !m.paused
	m.dirty = true
	if m.paused {
		m.liveNow = m.now
		m.pauseSource = m.snapshot
		m.snapshot = frozenSnapshot(m.snapshot)
		return
	}
	if !m.liveNow.IsZero() {
		m.now = m.liveNow
	}
	s := m.pauseSource
	if m.hasPending {
		s = m.pending
	}
	if s != nil {
		m.applySnapshot(s)
	}
	m.pauseSource = nil
	results := m.pausedResults
	m.pausedResults = pausedResults{}
	if results.payloadList != nil {
		m.applyPayloadList(*results.payloadList)
	}
	if results.blockList != nil {
		m.applyBlockList(*results.blockList)
	}
	if results.payloadDetail != nil {
		m.applyPayloadDetail(*results.payloadDetail)
	}
	if results.blockDetail != nil {
		m.applyBlockDetail(*results.blockDetail)
	}
	if results.blockDecision != nil {
		m.applyBlockDecision(*results.blockDecision)
	}
}

func frozenSnapshot(s *RuntimeSnapshot) *RuntimeSnapshot {
	if s == nil {
		return nil
	}
	frozen := *s
	if u := s.Usage; u != nil {
		copy := &remoteUsage{
			summaries: slices.Clone(u.Summaries()), recent: slices.Clone(u.Recent(recentLimit)),
			providers: slices.Clone(u.ProviderSummaries()), upstream: slices.Clone(u.UpstreamSummaries()),
			providerStatsAvailable: providerCountersAvailable(u),
		}
		if v, ok := u.(measurementViewer); ok {
			if r := v.RateSnapshot(); r != nil {
				rates := *r
				copy.rates = &rates
			}
			if b := v.BillingSnapshot(); b != nil {
				billing := *b
				billing.Usage = slices.Clone(b.Usage)
				billing.Upstream = slices.Clone(b.Upstream)
				copy.billing = &billing
				copy.summaries = billing.Usage
			}
		}
		frozen.Usage = copy
	} else {
		frozen.Usage = &remoteUsage{}
	}
	if s.Logs != nil {
		frozen.Logs = &remoteLogs{entries: slices.Clone(s.Logs.Since(1 << 30))}
	}
	if s.Health != nil {
		frozen.Health = &remoteHealth{states: s.Health.Snapshot()}
	}
	return &frozen
}
