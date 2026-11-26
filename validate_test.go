package multicam

import (
	"strings"
	"testing"
)

func TestValidateRejectsBrokenReferences(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Project)
		want   string
	}{
		{
			name:   "empty sequence id",
			mutate: func(p *Project) { p.Sequence.ID = "" },
			want:   "sequence has an empty id",
		},
		{
			name:   "zero duration",
			mutate: func(p *Project) { p.Sequence.MasterDurationSeconds = 0 },
			want:   "master_duration_seconds",
		},
		{
			name:   "negative tolerance",
			mutate: func(p *Project) { p.Sequence.ToleranceSeconds = -0.1 },
			want:   "tolerance_seconds",
		},
		{
			name:   "duplicate machine",
			mutate: func(p *Project) { p.Machines = append(p.Machines, Machine{ID: "cam-a"}) },
			want:   "duplicate machine id",
		},
		{
			name:   "lens unknown machine",
			mutate: func(p *Project) { p.Lenses[0].MachineID = "missing" },
			want:   "unknown machine",
		},
		{
			name:   "clip wrong sequence",
			mutate: func(p *Project) { p.Clips[0].SequenceID = "other" },
			want:   "references sequence",
		},
		{
			name:   "clip wrong lens owner",
			mutate: func(p *Project) { p.Clips[0].LensID = "lens-b" },
			want:   "uses lens",
		},
		{
			name:   "clip empty range",
			mutate: func(p *Project) { p.Clips[0].EndSeconds = p.Clips[0].StartSeconds },
			want:   "must end after it starts",
		},
		{
			name:   "observation unknown machine",
			mutate: func(p *Project) { p.Claps[0].Observations[0].MachineID = "missing" },
			want:   "unknown machine",
		},
		{
			name:   "observation wrong clip owner",
			mutate: func(p *Project) { p.Claps[0].Observations[0].ClipID = "b-0001" },
			want:   "owned by",
		},
		{
			name: "duplicate observation for machine",
			mutate: func(p *Project) {
				p.Claps[0].Observations = append(
					p.Claps[0].Observations,
					p.Claps[0].Observations[0],
				)
			},
			want: "multiple observations",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			project := sixCameraProject()
			test.mutate(&project)
			err := project.Validate()
			if err == nil {
				t.Fatal("Validate() error = nil, want error")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("Validate() error = %q, want substring %q", err, test.want)
			}
		})
	}
}

func TestDecodeProjectIsStrict(t *testing.T) {
	const valid = `{
		"sequence":{"id":"s","master_duration_seconds":10,"tolerance_seconds":0.1},
		"machines":[{"id":"m"}],
		"lenses":[{"id":"l","machine_id":"m"}],
		"clips":[{"id":"c","sequence_id":"s","machine_id":"m","lens_id":"l",
			"start_seconds":0,"end_seconds":10}],
		"claps":[{"id":"clap","master_seconds":0,
			"observations":[{"machine_id":"m","local_seconds":0}]}]
	}`

	if _, err := DecodeProject(strings.NewReader(valid)); err != nil {
		t.Fatalf("DecodeProject(valid) error = %v", err)
	}

	unknownField := strings.Replace(valid, `"tolerance_seconds":0.1`, `"tolerance":0.1`, 1)
	if _, err := DecodeProject(strings.NewReader(unknownField)); err == nil {
		t.Error("DecodeProject() accepted an unknown field")
	}

	if _, err := DecodeProject(strings.NewReader(valid + ` {}`)); err == nil {
		t.Error("DecodeProject() accepted multiple JSON values")
	}
}
