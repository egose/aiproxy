package dashboard

import (
	"context"

	"charm.land/bubbletea/v2"
)

type requestSlot struct {
	generation uint64
	cancel     context.CancelFunc
}

func (s *requestSlot) invalidate() {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.generation++
}

func (s *requestSlot) start(parent context.Context, command func(context.Context) tea.Cmd) tea.Cmd {
	s.invalidate()
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	generation := s.generation
	cmd := command(ctx)
	return func() tea.Msg {
		defer cancel()
		switch msg := cmd().(type) {
		case payloadListMsg:
			msg.generation = generation
			return msg
		case payloadDetailMsg:
			msg.generation = generation
			return msg
		case blockListMsg:
			msg.generation = generation
			return msg
		case blockDetailMsg:
			msg.generation = generation
			return msg
		case blockDecisionMsg:
			msg.generation = generation
			return msg
		default:
			return msg
		}
	}
}
