package main

import (
	"encoding/json"
	"fmt"
	"os"
)

const usage = `matchframe - trace timeline frames back to source media

usage:
  matchframe match <sequence.json> <pool.json>   match every clip by frame fingerprints
  matchframe map <sequence.json> <pool.json> <seqFrame>
                                                 map a timeline frame to source frame
  matchframe fingerprint <image...>              dHash of PNG/JPEG/GIF frame(s)

sequence.json: [{"id","name","seqIn","seqOut","srcIn","srcOut","firstHash","lastHash"}]
pool.json:     [{"path","name","frames":[{"frame","hash"}]}]

Matching never trusts the file name alone: a clip is resolved only when a
source's fingerprints confirm BOTH endpoint frames (after undoing the
clip's speed change). Clips with zero or several confirming sources are
reported as not uniquely matchable.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "match":
		err = cmdMatch(os.Args[2:])
	case "map":
		err = cmdMap(os.Args[2:])
	case "fingerprint":
		err = cmdFingerprint(os.Args[2:])
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func cmdMatch(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("match needs <sequence.json> <pool.json>")
	}
	var clips []Clip
	var pool []Source
	if err := loadJSON(args[0], &clips); err != nil {
		return err
	}
	if err := loadJSON(args[1], &pool); err != nil {
		return err
	}
	reports := matchAll(clips, pool)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(reports); err != nil {
		return err
	}
	bad := 0
	fmt.Fprintln(os.Stderr, "\nnot uniquely matchable:")
	for _, r := range reports {
		if r.Status != "unique" {
			bad++
			fmt.Fprintf(os.Stderr, "  %-8s %-16s status=%s candidates=%v\n",
				r.Clip.ID, r.Clip.Name, r.Status, r.Candidates)
		}
	}
	fmt.Fprintf(os.Stderr, "%d/%d clips not uniquely matched\n", bad, len(reports))
	return nil
}

func cmdMap(args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("map needs <sequence.json> <pool.json> <seqFrame>")
	}
	var clips []Clip
	var pool []Source
	if err := loadJSON(args[0], &clips); err != nil {
		return err
	}
	if err := loadJSON(args[1], &pool); err != nil {
		return err
	}
	var seq int64
	if _, err := fmt.Sscanf(args[2], "%d", &seq); err != nil {
		return fmt.Errorf("bad seqFrame: %w", err)
	}
	for _, c := range clips {
		src, err := c.MapSeqToSrc(seq)
		if err != nil {
			continue
		}
		cands, _ := matchCandidates(&c, pool)
		where := "UNRESOLVED"
		if len(cands) == 1 {
			where = cands[0].Path
		} else if len(cands) > 1 {
			where = fmt.Sprintf("AMBIGUOUS(%d)", len(cands))
		}
		fmt.Printf("seq %d -> clip %s (%s) src frame %d @ %s [rate %.3gx]\n",
			seq, c.ID, c.Name, src, where, c.rate())
		return nil
	}
	return fmt.Errorf("no clip covers sequence frame %d", seq)
}

func cmdFingerprint(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("fingerprint needs at least one image file")
	}
	for _, p := range args {
		h, err := FingerprintFile(p)
		if err != nil {
			return err
		}
		fmt.Printf("%016x  %s\n", h, p)
	}
	return nil
}
