package ripple

import "fmt"

func (tl *Timeline) uniqueID(prefix string) string {
	if tl.idSeq == 0 {
		tl.idSeq = len(tl.Clips) + len(tl.Transitions) + len(tl.Fades) + len(tl.Markers)
	}
	for {
		tl.idSeq++
		id := fmt.Sprintf("%s-%d", prefix, tl.idSeq)
		if _, ok := tl.ClipByID(id); !ok {
			return id
		}
	}
}

func (tl *Timeline) fragmentID(base, side string) string {
	for seq := 1; ; seq++ {
		id := fmt.Sprintf("%s__%s%d", base, side, seq)
		if _, ok := tl.ClipByID(id); !ok {
			return id
		}
	}
}
