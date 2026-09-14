package payloadlog

type Recorder interface {
	Record(Entry) error
	MaxBodyBytes() int
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
