package hdrframe

// BatchReport groups reports from multiple sample timelines.
type BatchReport struct {
	SchemaVersion int       `json:"schema_version"`
	Reports       []*Report `json:"reports"`
}

// AuditInputs audits every timeline as an independent case.
func AuditInputs(inputs ...Input) (*BatchReport, error) {
	batch := &BatchReport{SchemaVersion: 1, Reports: make([]*Report, 0, len(inputs))}
	for i := range inputs {
		report, err := Audit(inputs[i])
		if err != nil {
			return nil, err
		}
		batch.Reports = append(batch.Reports, report)
	}
	return batch, nil
}

// HasMismatch reports whether any case has an unexpected applied version.
func (b *BatchReport) HasMismatch() bool {
	for _, report := range b.Reports {
		if report.Summary.Mismatched > 0 {
			return true
		}
	}
	return false
}

// Text renders every report separated by a blank line.
func (b *BatchReport) Text() string {
	out := ""
	for i, report := range b.Reports {
		if i > 0 {
			out += "\n\n"
		}
		out += report.Text()
	}
	return out
}
