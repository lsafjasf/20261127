package checker

import "encoding/json"

// Status is the three-way outcome of a single contract item.
type Status string

const (
	// StatusPass means the measured value satisfies the rule.
	StatusPass Status = "PASS"
	// StatusFail means the measured value violates the rule.
	StatusFail Status = "FAIL"
	// StatusUnknown means the probe did not provide enough information to
	// decide. Missing tool support must never be reported as a failure.
	StatusUnknown Status = "UNKNOWN"
)

// ItemResult is the per-rule verdict.
type ItemResult struct {
	ID       string `json:"id"`
	Category string `json:"category"`
	Status   Status `json:"status"`
	Expected string `json:"expected"`
	Actual   string `json:"actual,omitempty"`
	Message  string `json:"message"`
}

// Report aggregates every item verdict for one file.
type Report struct {
	ContractID string       `json:"contractId"`
	Platform   string       `json:"platform"`
	File       string       `json:"file"`
	Overall    Status       `json:"overall"`
	Items      []ItemResult `json:"items"`
}

// Counts returns the number of results in each state.
func (r *Report) Counts() (pass, fail, unknown int) {
	for _, item := range r.Items {
		switch item.Status {
		case StatusPass:
			pass++
		case StatusFail:
			fail++
		case StatusUnknown:
			unknown++
		}
	}
	return pass, fail, unknown
}

// JSON renders the report as indented JSON.
func (r *Report) JSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}
