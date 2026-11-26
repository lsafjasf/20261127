package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	cc "github.com/lsafjasf/20261127"
)

func readSamplesReader(reader io.Reader, source string) ([]cc.Sample, error) {
	csvReader := csv.NewReader(reader)
	csvReader.Comment = '#'
	csvReader.FieldsPerRecord = 3
	csvReader.TrimLeadingSpace = true

	var samples []cc.Sample
	first := true
	for line := 1; ; line++ {
		record, readErr := csvReader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("%s, CSV line %d: %w", source, line, readErr)
		}
		if first {
			first = false
			if isHeader(record) {
				continue
			}
		}
		id := strings.TrimSpace(strings.TrimPrefix(record[0], "\ufeff"))
		a, parseErr := cc.ParseRGB24(record[1])
		if parseErr != nil {
			return nil, fmt.Errorf("%s, CSV line %d, converter A: %w", source, line, parseErr)
		}
		b, parseErr := cc.ParseRGB24(record[2])
		if parseErr != nil {
			return nil, fmt.Errorf("%s, CSV line %d, converter B: %w", source, line, parseErr)
		}
		samples = append(samples, cc.Sample{ID: id, A: a, B: b})
	}
	if len(samples) == 0 {
		return nil, fmt.Errorf("%s: no samples found in CSV", source)
	}
	return samples, nil
}

func isHeader(record []string) bool {
	first := normalizedColumn(record[0])
	second := normalizedColumn(record[1])
	third := normalizedColumn(record[2])
	validID := first == "id" || first == "sample" || first == "sample_id" || first == "name"
	validA := second == "converter_a" || second == "a" || second == "color_a" || second == "left"
	validB := third == "converter_b" || third == "b" || third == "color_b" || third == "right"
	return validID && validA && validB
}

func normalizedColumn(value string) string {
	value = strings.TrimSpace(strings.TrimPrefix(value, "\ufeff"))
	return strings.ToLower(value)
}
