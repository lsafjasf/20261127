package multicam

import (
	"encoding/json"
	"fmt"
	"io"
)

// DecodeProject reads one strict JSON project document. Unknown fields are
// rejected so metadata typos cannot silently alter the sync result.
func DecodeProject(r io.Reader) (Project, error) {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()

	var project Project
	if err := decoder.Decode(&project); err != nil {
		return Project{}, fmt.Errorf("decode project: %w", err)
	}

	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Project{}, fmt.Errorf("decode project: multiple JSON values")
		}
		return Project{}, fmt.Errorf("decode project trailing data: %w", err)
	}
	return project, nil
}
