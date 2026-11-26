package multicam

import "testing"

func TestMergeIntervalsPreservesRealGaps(t *testing.T) {
	got := mergeIntervals([]Interval{
		{StartSeconds: 120, EndSeconds: 180},
		{StartSeconds: 0, EndSeconds: 60},
		{StartSeconds: 50, EndSeconds: 70},
		{StartSeconds: 60, EndSeconds: 120},
	})
	assertIntervals(t, got, []Interval{
		{StartSeconds: 0, EndSeconds: 180},
	})
	// The zero-length gap above is adjacency; a real one-minute gap must split.
	got = mergeIntervals([]Interval{
		{StartSeconds: 0, EndSeconds: 60},
		{StartSeconds: 120, EndSeconds: 180},
	})
	assertIntervals(t, got, []Interval{
		{StartSeconds: 0, EndSeconds: 60},
		{StartSeconds: 120, EndSeconds: 180},
	})
}

func TestAlignmentAtToleranceIsAccepted(t *testing.T) {
	project := sixCameraProject()
	project.Claps = project.Claps[:1]
	project.Claps[0].Observations[5].ErrorSeconds = project.Sequence.ToleranceSeconds

	report, err := Analyze(project)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	camF := channelsByMachine(report)["cam-f"]
	assertStatus(t, camF, StatusAligned)
	if camF.Alignment == nil || !camF.Alignment.Applied {
		t.Error("alignment at the tolerance boundary was not applied")
	}
}
