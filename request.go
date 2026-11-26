package tearcheck

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// Request is the portable JSON input for precomputed observations.
type Request struct {
	Metadata     Metadata      `json:"metadata"`
	Observations []Observation `json:"observations"`
}

// AnalyzeRequest classifies a decoded JSON request.
func AnalyzeRequest(request Request) *Report {
	return AnalyzeObservations(request.Observations, request.Metadata)
}

// ReadRequestFile reads and decodes a JSON request.
func ReadRequestFile(path string) (Request, error) {
	file, err := os.Open(path)
	if err != nil {
		return Request{}, err
	}
	defer file.Close()
	return DecodeRequest(file)
}

// DecodeRequest decodes a JSON request.
func DecodeRequest(reader io.Reader) (Request, error) {
	var request Request
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return Request{}, fmt.Errorf("decode tearcheck request: %w", err)
	}
	return request, nil
}

// WriteReportJSON writes an indented stable JSON report.
func WriteReportJSON(writer io.Writer, report *Report) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

// MarshalReport returns indented report JSON.
func MarshalReport(report *Report) ([]byte, error) {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
