package modelresolver

import (
	"sync"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/provider"
)

type CooldownFingerprint struct {
	Alias         string
	Provider      string
	Model         string
	BaseURL       string
	Credential    string
	UpstreamModel string
	Protocol      string
}

func CooldownFingerprintFor(alias string, prov config.Provider, model config.Model) CooldownFingerprint {
	credential := "key:" + prov.APIKey
	if prov.CopilotToken != "" {
		credential = "copilot:" + prov.CopilotToken
	}
	return CooldownFingerprint{
		Alias:         alias,
		Provider:      prov.Name,
		Model:         model.Name,
		BaseURL:       provider.EffectiveBaseURL(prov.Type, prov.BaseURL),
		Credential:    credential,
		UpstreamModel: model.UpstreamName,
		Protocol:      string(model.Protocol),
	}
}

type ActiveCooldown struct {
	Alias     string
	Provider  string
	Model     string
	Remaining time.Duration
}

type CooldownStore struct {
	mu        sync.Mutex
	deadlines map[CooldownFingerprint]time.Time
	now       func() time.Time
}

func (s *CooldownStore) Active() []ActiveCooldown {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	out := make([]ActiveCooldown, 0, len(s.deadlines))
	for fp, deadline := range s.deadlines {
		remaining := deadline.Sub(now)
		if remaining <= 0 {
			delete(s.deadlines, fp)
			continue
		}
		out = append(out, ActiveCooldown{
			Alias:     fp.Alias,
			Provider:  fp.Provider,
			Model:     fp.Model,
			Remaining: remaining,
		})
	}
	return out
}

func NewCooldownStore() *CooldownStore {
	return &CooldownStore{
		deadlines: make(map[CooldownFingerprint]time.Time),
		now:       time.Now,
	}
}

func (s *CooldownStore) SetNowFunc(fn func() time.Time) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if fn == nil {
		s.now = time.Now
		return
	}
	s.now = fn
}

func (s *CooldownStore) currentTime() time.Time {
	if s == nil {
		return time.Now()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.now()
}

func (s *CooldownStore) Observe(fp CooldownFingerprint, delay time.Duration) {
	if s == nil || delay <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	deadline := s.now().Add(delay)
	if existing, ok := s.deadlines[fp]; ok && !deadline.After(existing) {
		return
	}
	s.deadlines[fp] = deadline
}

func (s *CooldownStore) Remaining(fp CooldownFingerprint) (time.Duration, bool) {
	if s == nil {
		return 0, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	deadline, ok := s.deadlines[fp]
	if !ok {
		return 0, false
	}
	remaining := deadline.Sub(s.now())
	if remaining <= 0 {
		delete(s.deadlines, fp)
		return 0, false
	}
	return remaining, true
}

func (s *CooldownStore) cloneForCatalog(catalog config.Catalog) *CooldownStore {
	next := NewCooldownStore()
	if s == nil {
		return next
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next.now = s.now
	now := s.now()
	valid := validCooldownFingerprints(catalog)
	for fp, deadline := range s.deadlines {
		if !valid[fp] {
			continue
		}
		if deadline.Sub(now) <= 0 {
			continue
		}
		next.deadlines[fp] = deadline
	}
	return next
}

func validCooldownFingerprints(catalog config.Catalog) map[CooldownFingerprint]bool {
	valid := make(map[CooldownFingerprint]bool)
	for _, a := range catalog.Aliases() {
		for _, target := range a.Targets {
			prov, model, ok := catalog.Model(target.Provider, target.Model)
			if !ok {
				continue
			}
			valid[CooldownFingerprintFor(a.Name, prov, model)] = true
		}
	}
	return valid
}
