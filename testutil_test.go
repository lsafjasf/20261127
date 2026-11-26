package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
)

// buildHashTrace builds an n-frame smooth hash chain. jumps overrides the
// incoming-edge flip count at given frame indexes; dupAt makes an exact copy.
func buildHashTrace(n int, fps float64, seed int64, jumps map[int]int, dupAt map[int]bool) *Trace {
	rng := rand.New(rand.NewSource(seed + 7))
	frames := make([]*Frame, 0, n)
	var h uint64 = rng.Uint64() | 1
	for i := 0; i < n; i++ {
		if i > 0 {
			switch {
			case dupAt[i]:
				// same hash
			default:
				bits := 3
				if b, ok := jumps[i]; ok {
					bits = b
				}
				h = flipBits(h, bits, rng)
			}
		}
		frames = append(frames, &Frame{
			Stage: StageRecord, Line: i + 1,
			Seq: int64(i), HasSeq: true,
			PTS: float64(i) / fps, HasPTS: true,
			Sig: Sig{Kind: SigPHash64, P: h},
		})
	}
	return &Trace{Stage: StageRecord, Source: "memory", Frames: frames}
}

func writeTraceJSONL(path string, tr *Trace) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, fr := range tr.Frames {
		m := map[string]any{"seq": fr.Seq, "pts": fr.PTS}
		if fr.Sig.Kind == SigPHash64 {
			m["hash"] = fmt.Sprintf("%016x", fr.Sig.P)
		}
		if err := enc.Encode(m); err != nil {
			return err
		}
	}
	return nil
}
