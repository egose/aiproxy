package payloadlog

import "context"

type Recorder interface {
	Record(Entry) error
	MaxBodyBytes() int
}

type ContextRecorder interface {
	Recorder
	RecordContext(context.Context, Entry) error
}

func RecordContext(ctx context.Context, recorder Recorder, entry Entry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if recorder == nil {
		return nil
	}
	if recorder, ok := recorder.(ContextRecorder); ok {
		return recorder.RecordContext(ctx, entry)
	}
	return recorder.Record(entry)
}

type fanout struct {
	members []Recorder
}

func Combine(recorders ...Recorder) Recorder {
	var members []Recorder
	for _, r := range recorders {
		if r == nil {
			continue
		}
		members = append(members, r)
	}
	switch len(members) {
	case 0:
		return nil
	case 1:
		return members[0]
	default:
		return &fanout{members: members}
	}
}

func (m *fanout) MaxBodyBytes() int {
	min := 0
	for _, r := range m.members {
		if n := r.MaxBodyBytes(); n > 0 && (min == 0 || n < min) {
			min = n
		}
	}
	return min
}

func (m *fanout) Record(e Entry) error {
	var first error
	for _, r := range m.members {
		if err := r.Record(e); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (m *fanout) RecordContext(ctx context.Context, e Entry) error {
	var first error
	for _, r := range m.members {
		if err := RecordContext(ctx, r, e); err != nil && first == nil {
			first = err
		}
	}
	return first
}
