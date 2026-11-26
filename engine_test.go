package multicam

import (
	"math"
	"strings"
	"testing"
)

func TestAnalyzeSixCamerasMappingAndCoverage(t *testing.T) {
	project := sixCameraProject()
	report, err := Analyze(project)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if len(report.Channels) != 6 {
		t.Fatalf("got %d channels, want 6", len(report.Channels))
	}

	byMachine := channelsByMachine(report)
	assertStatus(t, byMachine["cam-a"], StatusAligned)
	assertIntervals(t, byMachine["cam-a"].Coverage, []Interval{
		{StartSeconds: 0, EndSeconds: 180},
	})

	assertIntervals(t, byMachine["cam-b"].Coverage, []Interval{
		{StartSeconds: 0, EndSeconds: 60},
		{StartSeconds: 120, EndSeconds: 180},
	})
	if got := byMachine["cam-b"].CoveredSeconds; !closeEnough(got, 120) {
		t.Errorf("cam-b covered_seconds = %v, want 120", got)
	}

	camD := byMachine["cam-d"]
	if len(camD.Lenses) != 2 {
		t.Fatalf("cam-d has %d lens layers, want 2", len(camD.Lenses))
	}
	if got := camD.Lenses[0].Clips[0].ID; got != "d-0001" {
		t.Errorf("cam-d first clip = %q, want d-0001", got)
	}
	assertIntervals(t, camD.Coverage, []Interval{
		{StartSeconds: 0, EndSeconds: 180},
	})

	for _, machineID := range []string{"cam-c", "cam-e", "cam-f"} {
		assertIntervals(t, byMachine[machineID].Coverage, []Interval{
			{StartSeconds: 0, EndSeconds: 180},
		})
	}
}

func TestAnalyzeRejectsOutlierWithoutApplyingTeamOffset(t *testing.T) {
	project := sixCameraProject()
	project.Claps[1].Observations[5].LocalSeconds = 2625

	report, err := Analyze(project)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	camF := channelsByMachine(report)["cam-f"]
	assertStatus(t, camF, StatusToleranceExceeded)
	if camF.Alignment == nil {
		t.Fatal("cam-f alignment report is nil")
	}
	if camF.Alignment.Applied {
		t.Error("cam-f alignment was applied despite exceeding tolerance")
	}
	if len(camF.Coverage) != 0 {
		t.Errorf("cam-f coverage = %v, want none", camF.Coverage)
	}
	for _, lens := range camF.Lenses {
		for _, clip := range lens.Clips {
			if clip.MasterInterval != nil {
				t.Errorf("cam-f clip %q has a master interval despite rejection", clip.ID)
			}
			if clip.CoverageInterval != nil {
				t.Errorf("cam-f clip %q has a coverage interval despite rejection", clip.ID)
			}
		}
	}
	if !strings.Contains(camF.RejectionReason, "team offset was not applied") {
		t.Errorf("rejection reason %q does not document the no-fallback rule", camF.RejectionReason)
	}
	if camF.Alignment.MaxErrorSeconds <= project.Sequence.ToleranceSeconds {
		t.Errorf("max error %v should exceed tolerance %v",
			camF.Alignment.MaxErrorSeconds, project.Sequence.ToleranceSeconds)
	}
}

func TestAnalyzeClipsToMasterRangeAndDropsOutsideClips(t *testing.T) {
	project := Project{
		Sequence: Sequence{
			ID:                    "seq",
			MasterDurationSeconds: 100,
			ToleranceSeconds:      0.001,
		},
		Machines: []Machine{{ID: "cam"}},
		Lenses:   []Lens{{ID: "lens", MachineID: "cam"}},
		Clips: []Clip{
			{
				ID: "partial", SequenceID: "seq", MachineID: "cam", LensID: "lens",
				StartSeconds: 95, EndSeconds: 115,
			},
			{
				ID: "outside", SequenceID: "seq", MachineID: "cam", LensID: "lens",
				StartSeconds: 200, EndSeconds: 210,
			},
		},
		Claps: []Clap{{
			ID: "clap", MasterSeconds: 0,
			Observations: []ClapObservation{{
				MachineID: "cam", LocalSeconds: 100,
			}},
		}},
	}

	report, err := Analyze(project)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	channel := report.Channels[0]
	assertIntervals(t, channel.Coverage, []Interval{{StartSeconds: 0, EndSeconds: 15}})

	clips := channel.Lenses[0].Clips
	if clips[0].MasterInterval == nil || !closeEnough(clips[0].MasterInterval.StartSeconds, -5) {
		t.Errorf("partial clip master interval = %+v, want start -5", clips[0].MasterInterval)
	}
	if clips[1].MasterInterval == nil || clips[1].CoverageInterval != nil {
		t.Errorf("outside clip should be mapped but have no coverage: %+v", clips[1])
	}
}

func TestAnalyzeMissingClapAndNoClips(t *testing.T) {
	project := sixCameraProject()
	project.Claps = nil
	project.Machines = append(project.Machines, Machine{ID: "cam-g"})
	project.Lenses = append(project.Lenses, Lens{ID: "lens-g", MachineID: "cam-g"})

	report, err := Analyze(project)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	byMachine := channelsByMachine(report)
	if len(byMachine["cam-a"].Coverage) != 0 {
		t.Errorf("cam-a coverage = %v, want none", byMachine["cam-a"].Coverage)
	}
	assertStatus(t, byMachine["cam-a"], StatusMissingClap)
	assertStatus(t, byMachine["cam-g"], StatusNoClips)
}

func sixCameraProject() Project {
	machines := []Machine{
		{ID: "cam-a"}, {ID: "cam-b"}, {ID: "cam-c"},
		{ID: "cam-d"}, {ID: "cam-e"}, {ID: "cam-f"},
	}
	lenses := []Lens{
		{ID: "lens-a", MachineID: "cam-a"},
		{ID: "lens-b", MachineID: "cam-b"},
		{ID: "lens-c", MachineID: "cam-c"},
		{ID: "lens-d-1", MachineID: "cam-d"},
		{ID: "lens-d-2", MachineID: "cam-d"},
		{ID: "lens-e", MachineID: "cam-e"},
		{ID: "lens-f", MachineID: "cam-f"},
	}
	clips := []Clip{
		clip("a-0001", "cam-a", "lens-a", 50, 110),
		clip("a-0002", "cam-a", "lens-a", 110, 230),
		clip("b-0001", "cam-b", "lens-b", 500, 560),
		clip("b-0002", "cam-b", "lens-b", 620, 680),
		clip("c-0001", "cam-c", "lens-c", 1000, 1300),
		clip("d-0001", "cam-d", "lens-d-1", 1500, 1620),
		clip("d-0002", "cam-d", "lens-d-2", 1620, 1800),
		clip("e-0001", "cam-e", "lens-e", 2000, 2300),
		clip("f-0001", "cam-f", "lens-f", 2500, 2800),
	}
	claps := []Clap{
		{
			ID: "clap-1", MasterSeconds: 0,
			Observations: []ClapObservation{
				{MachineID: "cam-a", LocalSeconds: 50},
				{MachineID: "cam-b", LocalSeconds: 500},
				{MachineID: "cam-c", LocalSeconds: 1000},
				{MachineID: "cam-d", LocalSeconds: 1500},
				{MachineID: "cam-e", LocalSeconds: 2000},
				{MachineID: "cam-f", LocalSeconds: 2500},
			},
		},
		{
			ID: "clap-2", MasterSeconds: 120,
			Observations: []ClapObservation{
				{MachineID: "cam-a", LocalSeconds: 170},
				{MachineID: "cam-b", LocalSeconds: 620},
				{MachineID: "cam-c", LocalSeconds: 1120},
				{MachineID: "cam-d", LocalSeconds: 1620},
				{MachineID: "cam-e", LocalSeconds: 2120},
				{MachineID: "cam-f", LocalSeconds: 2620},
			},
		},
	}
	return Project{
		Sequence: Sequence{
			ID:                    "seq",
			MasterDurationSeconds: 180,
			ToleranceSeconds:      0.05,
		},
		Machines: machines,
		Lenses:   lenses,
		Clips:    clips,
		Claps:    claps,
	}
}

func clip(id, machineID, lensID string, start, end float64) Clip {
	return Clip{
		ID: id, SequenceID: "seq", MachineID: machineID, LensID: lensID,
		StartSeconds: start, EndSeconds: end,
	}
}

func channelsByMachine(report Report) map[string]ChannelReport {
	result := make(map[string]ChannelReport, len(report.Channels))
	for _, channel := range report.Channels {
		result[channel.Machine.ID] = channel
	}
	return result
}

func assertStatus(t *testing.T, channel ChannelReport, want string) {
	t.Helper()
	if channel.Status != want {
		t.Errorf("%s status = %q, want %q", channel.Machine.ID, channel.Status, want)
	}
}

func assertIntervals(t *testing.T, got, want []Interval) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("intervals = %v, want %v", got, want)
	}
	for i := range want {
		if !closeEnough(got[i].StartSeconds, want[i].StartSeconds) ||
			!closeEnough(got[i].EndSeconds, want[i].EndSeconds) {
			t.Errorf("interval %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func closeEnough(got, want float64) bool {
	return math.Abs(got-want) <= 1e-9
}
