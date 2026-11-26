package ripple

import (
	"math"
	"testing"
)

func TestRandomTwentyTrace(t *testing.T) {
	for _, mode := range []Mode{ModeInsert, ModeOverwrite} {
		result, err := RunRandom(20261127, 20, mode, MarkerPolicyRipple)
		if err != nil {
			t.Fatalf("%s random run: %v", mode, err)
		}
		if !result.Verification.OK {
			t.Fatalf("%s verification failed: %v", mode, result.Verification.Errors)
		}
		if len(result.Audit.Records) != 20 {
			t.Fatalf("%s got %d records", mode, len(result.Audit.Records))
		}
	}
}

func TestManySeedsStayValid(t *testing.T) {
	for seed := int64(1); seed <= 80; seed++ {
		for _, mode := range []Mode{ModeInsert, ModeOverwrite} {
			result, err := RunRandom(seed, 20, mode, MarkerPolicyRipple)
			if err != nil {
				t.Fatalf("seed %d mode %s: %v", seed, mode, err)
			}
			if !result.Verification.OK {
				t.Fatalf("seed %d mode %s verification: %v", seed, mode, result.Verification.Errors)
			}
		}
	}
}

func TestRandomScriptIsDeterministic(t *testing.T) {
	first, err := RunRandom(99, 20, ModeInsert, MarkerPolicyRipple)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RunRandom(99, 20, ModeInsert, MarkerPolicyRipple)
	if err != nil {
		t.Fatal(err)
	}
	if first.Timeline.Duration() != second.Timeline.Duration() {
		t.Fatal("same seed produced different durations")
	}
	for i := range first.Audit.Records {
		if first.Audit.Records[i].AfterDuration != second.Audit.Records[i].AfterDuration {
			t.Fatalf("op %d is not reproducible", i)
		}
	}
}

func TestInsertRipplesAllBindings(t *testing.T) {
	timeline := DemoTimeline()
	audit := newAudit(ModeInsert, 1, 1, timeline.Duration())
	audit.InitMarkerTraces(timeline)
	_, err := Apply(&timeline, Edit{
		Index: 0, Mode: ModeInsert, At: 4, Duration: 3,
		NewClipID: "drag-x", TargetTrackID: "V1", MarkerPolicy: MarkerPolicyRipple,
	}, audit)
	if err != nil {
		t.Fatal(err)
	}

	if timeline.Duration() != 27 {
		t.Fatalf("insert duration = %d, want 27", timeline.Duration())
	}
	assertClipStart(t, timeline, "v-interview", 11)
	assertClipStart(t, timeline, "v-outro", 19)
	assertClipStart(t, timeline, "a-interview", 11)
	assertClipStart(t, timeline, "a-outro", 19)
	assertMarkerTime(t, timeline, "mk-cut-8", 11)
	assertMarkerTime(t, timeline, "mk-cut-16", 19)
	assertMarkerTime(t, timeline, "mk-end", 27)
	assertKeyframeTime(t, timeline, "kf-a1-4", 7)
	assertKeyframeTime(t, timeline, "kf-a1-12", 15)

	layout, err := Recompute(timeline)
	if err != nil {
		t.Fatal(err)
	}
	transition := findResolvedTransition(t, layout, "tr-v-01")
	if transition.Boundary != 11 || transition.Start != 10 || transition.End != 12 {
		t.Fatalf("transition 01 = boundary %d interval %d-%d", transition.Boundary, transition.Start, transition.End)
	}
	transition2 := findResolvedTransition(t, layout, "tr-v-02")
	if transition2.Boundary != 19 || transition2.Start != 18 {
		t.Fatalf("transition 02 = boundary %d start %d", transition2.Boundary, transition2.Start)
	}
	fadeOut := findResolvedFade(t, layout, "fade-a-intro-out")
	if fadeOut.Interval.Start != 10 || fadeOut.Interval.End != 11 {
		t.Fatalf("rippled fade-out = %d-%d, want 10-11", fadeOut.Interval.Start, fadeOut.Interval.End)
	}

	envelope := findEnvelope(t, timeline, "env-a2-volume")
	wantTimes := []Tick{0, 4, 7, 9, 17, 27}
	if len(envelope.Points) != len(wantTimes) {
		t.Fatalf("envelope points = %v, want times %v", envelope.Points, wantTimes)
	}
	for i, wantTime := range wantTimes {
		if envelope.Points[i].Time != wantTime {
			t.Fatalf("envelope point %d time = %d, want %d", i, envelope.Points[i].Time, wantTime)
		}
	}
	if math.Abs(envelope.Points[1].Value-envelope.Points[2].Value) > 1e-9 {
		t.Fatalf("split hold values differ: %v vs %v", envelope.Points[1], envelope.Points[2])
	}
	if !VerifyTrace(timeline, audit).OK {
		t.Fatalf("trace invalid: %v", VerifyTrace(timeline, audit).Errors)
	}
}

func TestOverwriteDoesNotRippleDownstream(t *testing.T) {
	timeline := DemoTimeline()
	audit := newAudit(ModeOverwrite, 1, 1, timeline.Duration())
	audit.InitMarkerTraces(timeline)
	_, err := Apply(&timeline, Edit{
		Index: 0, Mode: ModeOverwrite, At: 4, Duration: 3,
		NewClipID: "replace-a", TargetTrackID: "A1",
	}, audit)
	if err != nil {
		t.Fatal(err)
	}

	if timeline.Duration() != 24 {
		t.Fatalf("overwrite changed duration to %d", timeline.Duration())
	}
	assertClipStart(t, timeline, "a-interview", 8)
	assertClipStart(t, timeline, "a-outro", 16)
	assertMarkerTime(t, timeline, "mk-cut-8", 8)
	assertMarkerTime(t, timeline, "mk-cut-16", 16)
	assertKeyframeTime(t, timeline, "kf-a1-4", 4)

	layout, err := Recompute(timeline)
	if err != nil {
		t.Fatal(err)
	}
	fadeOut := findResolvedFade(t, layout, "fade-a-intro-out")
	if fadeOut.Interval.Start != 7 || fadeOut.Interval.End != 8 {
		t.Fatalf("overwrite fade-out moved to %d-%d", fadeOut.Interval.Start, fadeOut.Interval.End)
	}
	if !VerifyTrace(timeline, audit).OK {
		t.Fatalf("trace invalid: %v", VerifyTrace(timeline, audit).Errors)
	}
}

func TestTrackLocalInsertReportsLinkedDesync(t *testing.T) {
	timeline := DemoTimeline()
	_, err := Apply(&timeline, Edit{
		Index: 1, Mode: ModeInsert, At: 4, Duration: 2,
		NewClipID: "video-only", TargetTrackID: "V1",
		RippleTrackIDs: []string{"V1"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	assertClipStart(t, timeline, "v-interview", 10)
	assertClipStart(t, timeline, "a-interview", 8)
	issues := CheckLinkedSync(timeline)
	if len(issues) == 0 {
		t.Fatal("expected linked sync issue for track-local ripple")
	}
}

func assertClipStart(t *testing.T, tl Timeline, id string, want Tick) {
	t.Helper()
	clip, ok := tl.ClipByID(id)
	if !ok {
		t.Fatalf("missing clip %s", id)
	}
	if clip.Start != want {
		t.Fatalf("clip %s start = %d, want %d", id, clip.Start, want)
	}
}

func assertMarkerTime(t *testing.T, tl Timeline, id string, want Tick) {
	t.Helper()
	for _, marker := range tl.Markers {
		if marker.ID == id {
			if marker.Time != want {
				t.Fatalf("marker %s time = %d, want %d", id, marker.Time, want)
			}
			return
		}
	}
	t.Fatalf("missing marker %s", id)
}

func assertKeyframeTime(t *testing.T, tl Timeline, id string, want Tick) {
	t.Helper()
	for _, keyframe := range tl.Keyframes {
		if keyframe.ID == id {
			if keyframe.Time != want {
				t.Fatalf("keyframe %s time = %d, want %d", id, keyframe.Time, want)
			}
			return
		}
	}
	t.Fatalf("missing keyframe %s", id)
}

func findResolvedTransition(t *testing.T, layout Layout, id string) ResolvedTransition {
	t.Helper()
	for _, transition := range layout.Transitions {
		if transition.ID == id {
			return transition
		}
	}
	t.Fatalf("missing transition %s", id)
	return ResolvedTransition{}
}

func findResolvedFade(t *testing.T, layout Layout, id string) ResolvedFade {
	t.Helper()
	for _, fade := range layout.Fades {
		if fade.ID == id {
			return fade
		}
	}
	t.Fatalf("missing fade %s", id)
	return ResolvedFade{}
}

func findEnvelope(t *testing.T, tl Timeline, id string) Envelope {
	t.Helper()
	for _, envelope := range tl.Envelopes {
		if envelope.ID == id {
			return envelope
		}
	}
	t.Fatalf("missing envelope %s", id)
	return Envelope{}
}
