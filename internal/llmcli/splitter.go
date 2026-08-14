package llmcli

import "strings"

const (
	thinkOpen  = "<think>"
	thinkClose = "</think>"
)

type SegmentKind int

const (
	SegmentContent SegmentKind = iota
	SegmentThinking
)

// Segment — фрагмент текста контента или рассуждений.
type Segment struct {
	Kind SegmentKind
	Text string
}

// ThinkSplitter разбирает поток токенов на сегменты контента и рассуждений
// (<think>...</think>). Устойчив к тегам, разбитым между токенами.
type ThinkSplitter struct {
	hold string
	mode bool
}

// Feed разбирает очередной фрагмент текста на сегменты.
func (s *ThinkSplitter) Feed(text string) []Segment {
	segments := make([]Segment, 0, 1)
	pending := s.hold + text
	s.hold = ""

	for pending != "" {
		if s.mode {
			idx := strings.Index(pending, thinkClose)
			if idx < 0 {
				if cut := partialSuffix(pending, thinkClose); cut > 0 {
					segments = append(segments, Segment{SegmentThinking, pending[:len(pending)-cut]})
					s.hold = pending[len(pending)-cut:]
				} else {
					segments = append(segments, Segment{SegmentThinking, pending})
				}
				return segments
			}
			segments = append(segments, Segment{SegmentThinking, pending[:idx]})
			s.mode = false
			pending = pending[idx+len(thinkClose):]
			continue
		}

		idx := strings.Index(pending, thinkOpen)
		if idx < 0 {
			if cut := partialSuffix(pending, thinkOpen); cut > 0 {
				segments = append(segments, Segment{SegmentContent, pending[:len(pending)-cut]})
				s.hold = pending[len(pending)-cut:]
			} else {
				segments = append(segments, Segment{SegmentContent, pending})
			}
			return segments
		}
		segments = append(segments, Segment{SegmentContent, pending[:idx]})
		s.mode = true
		pending = pending[idx+len(thinkOpen):]
	}
	return segments
}

// Flush возвращает оставшийся задержанный хвост и сбрасывает буфер.
func (s *ThinkSplitter) Flush() []Segment {
	if s.hold == "" {
		return nil
	}
	kind := SegmentContent
	if s.mode {
		kind = SegmentThinking
	}
	seg := Segment{kind, s.hold}
	s.hold = ""
	return []Segment{seg}
}

// partialSuffix возвращает длину наибольшего суффикса s, являющегося префиксом tag.
func partialSuffix(s, tag string) int {
	max := len(tag) - 1
	if len(s) < max {
		max = len(s)
	}
	for n := max; n > 0; n-- {
		if strings.HasPrefix(tag, s[len(s)-n:]) {
			return n
		}
	}
	return 0
}
