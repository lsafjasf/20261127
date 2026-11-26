package multicam

import (
	"fmt"
	"math"
)

// Validate checks referential integrity and all values required for timeline
// projection. Alignment quality is checked later by Analyze.
func (p Project) Validate() error {
	if err := validateSequence(p.Sequence); err != nil {
		return err
	}
	if len(p.Machines) == 0 {
		return fmt.Errorf("at least one machine is required")
	}

	machines := make(map[string]Machine, len(p.Machines))
	for i, machine := range p.Machines {
		if err := requireID("machine", machine.ID, i); err != nil {
			return err
		}
		if _, exists := machines[machine.ID]; exists {
			return fmt.Errorf("duplicate machine id %q", machine.ID)
		}
		machines[machine.ID] = machine
	}

	lenses := make(map[string]Lens, len(p.Lenses))
	for i, lens := range p.Lenses {
		if err := requireID("lens", lens.ID, i); err != nil {
			return err
		}
		if _, exists := lenses[lens.ID]; exists {
			return fmt.Errorf("duplicate lens id %q", lens.ID)
		}
		if _, exists := machines[lens.MachineID]; !exists {
			return fmt.Errorf("lens %q references unknown machine %q", lens.ID, lens.MachineID)
		}
		if !isFinite(lens.FocalLengthMM) || lens.FocalLengthMM < 0 {
			return fmt.Errorf("lens %q has invalid focal_length_mm", lens.ID)
		}
		lenses[lens.ID] = lens
	}

	clips := make(map[string]Clip, len(p.Clips))
	for i, clip := range p.Clips {
		if err := requireID("clip", clip.ID, i); err != nil {
			return err
		}
		if _, exists := clips[clip.ID]; exists {
			return fmt.Errorf("duplicate clip id %q", clip.ID)
		}
		if clip.SequenceID != p.Sequence.ID {
			return fmt.Errorf("clip %q references sequence %q, want %q", clip.ID, clip.SequenceID, p.Sequence.ID)
		}
		if _, exists := machines[clip.MachineID]; !exists {
			return fmt.Errorf("clip %q references unknown machine %q", clip.ID, clip.MachineID)
		}
		lens, exists := lenses[clip.LensID]
		if !exists {
			return fmt.Errorf("clip %q references unknown lens %q", clip.ID, clip.LensID)
		}
		if lens.MachineID != clip.MachineID {
			return fmt.Errorf(
				"clip %q uses lens %q mounted on machine %q, not %q",
				clip.ID, clip.LensID, lens.MachineID, clip.MachineID,
			)
		}
		if !isFinite(clip.StartSeconds) || !isFinite(clip.EndSeconds) {
			return fmt.Errorf("clip %q has non-finite start or end time", clip.ID)
		}
		if clip.EndSeconds <= clip.StartSeconds {
			return fmt.Errorf("clip %q must end after it starts", clip.ID)
		}
		clips[clip.ID] = clip
	}

	claps := make(map[string]struct{}, len(p.Claps))
	for i, clap := range p.Claps {
		if err := requireID("clap", clap.ID, i); err != nil {
			return err
		}
		if _, exists := claps[clap.ID]; exists {
			return fmt.Errorf("duplicate clap id %q", clap.ID)
		}
		claps[clap.ID] = struct{}{}
		if !isFinite(clap.MasterSeconds) {
			return fmt.Errorf("clap %q has non-finite master_seconds", clap.ID)
		}
		if len(clap.Observations) == 0 {
			return fmt.Errorf("clap %q has no observations", clap.ID)
		}
		seenMachines := make(map[string]struct{}, len(clap.Observations))
		for j, observation := range clap.Observations {
			if _, exists := machines[observation.MachineID]; !exists {
				return fmt.Errorf(
					"clap %q observation %d references unknown machine %q",
					clap.ID, j, observation.MachineID,
				)
			}
			if _, exists := seenMachines[observation.MachineID]; exists {
				return fmt.Errorf(
					"clap %q has multiple observations for machine %q",
					clap.ID, observation.MachineID,
				)
			}
			seenMachines[observation.MachineID] = struct{}{}
			if !isFinite(observation.LocalSeconds) {
				return fmt.Errorf("clap %q has non-finite local_seconds", clap.ID)
			}
			if !isFinite(observation.ErrorSeconds) || observation.ErrorSeconds < 0 {
				return fmt.Errorf("clap %q has invalid error_seconds", clap.ID)
			}
			if observation.ClipID == "" {
				continue
			}
			clip, exists := clips[observation.ClipID]
			if !exists {
				return fmt.Errorf(
					"clap %q references unknown clip %q",
					clap.ID, observation.ClipID,
				)
			}
			if clip.MachineID != observation.MachineID {
				return fmt.Errorf(
					"clap %q observation for machine %q references clip %q owned by %q",
					clap.ID, observation.MachineID, clip.ID, clip.MachineID,
				)
			}
		}
	}

	return nil
}

func validateSequence(sequence Sequence) error {
	if err := requireID("sequence", sequence.ID, -1); err != nil {
		return err
	}
	if !isFinite(sequence.MasterDurationSeconds) || sequence.MasterDurationSeconds <= 0 {
		return fmt.Errorf("master_duration_seconds must be greater than zero")
	}
	if !isFinite(sequence.ToleranceSeconds) || sequence.ToleranceSeconds < 0 {
		return fmt.Errorf("tolerance_seconds must be zero or greater")
	}
	return nil
}

func requireID(kind, id string, index int) error {
	if id != "" {
		return nil
	}
	if index >= 0 {
		return fmt.Errorf("%s at index %d has an empty id", kind, index)
	}
	return fmt.Errorf("%s has an empty id", kind)
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
