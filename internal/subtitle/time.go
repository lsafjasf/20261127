package subtitle

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseTime accepts SRT ("HH:MM:SS,mmm") and WebVTT ("HH:MM:SS.mmm",
// optionally "MM:SS.mmm") timestamps and returns an exact time in
// nanoseconds so every deliverable shares one timing representation.
func ParseTime(value string) (int64, error) {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return 0, fmt.Errorf("empty timestamp")
	}
	fracSep := byte(',')
	if strings.ContainsRune(raw, '.') {
		fracSep = '.'
	}

	var head, frac string
	if idx := strings.IndexByte(raw, fracSep); idx >= 0 {
		head, frac = raw[:idx], raw[idx+1:]
	} else {
		head = raw
	}

	parts := strings.Split(head, ":")
	if len(parts) < 1 || len(parts) > 3 {
		return 0, fmt.Errorf("invalid timestamp %q", value)
	}
	// SRT timestamps always use the full HH:MM:SS,mmm form. VTT may also
	// use MM:SS.mmm or SS.mmm short forms.
	if fracSep == ',' && len(parts) != 3 {
		return 0, fmt.Errorf("invalid timestamp %q", value)
	}
	if len(parts) == 1 && fracSep != '.' {
		return 0, fmt.Errorf("invalid timestamp %q", value)
	}
	values := make([]int64, 3)
	offset := 3 - len(parts)
	for i, part := range parts {
		n, err := strconv.ParseInt(part, 10, 64)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("invalid timestamp %q", value)
		}
		values[offset+i] = n
	}
	hour, minute, second := values[0], values[1], values[2]
	if minute > 59 || second > 59 {
		return 0, fmt.Errorf("invalid timestamp %q", value)
	}

	nanos := ((hour*60+minute)*60 + second) * 1_000_000_000
	if frac != "" {
		if len(frac) > 9 {
			return 0, fmt.Errorf("sub-nanosecond precision in %q", value)
		}
		f, err := strconv.ParseInt(frac, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid timestamp %q", value)
		}
		for i := len(frac); i < 9; i++ {
			f *= 10
		}
		nanos += f
	}
	return nanos, nil
}

// FormatSRT renders nanoseconds as an SRT timestamp (millisecond precision).
func FormatSRT(nanos int64) string {
	return format(nanos, 3, ',')
}

// FormatVTT renders nanoseconds as a WebVTT timestamp (millisecond precision).
func FormatVTT(nanos int64) string {
	return format(nanos, 3, '.')
}

// FormatESUB renders nanoseconds with full nanosecond precision for the
// embedded container, so timing comparison never loses resolution.
func FormatESUB(nanos int64) string {
	return format(nanos, 9, '.')
}

func format(nanos int64, fracDigits int, sep byte) string {
	if nanos < 0 {
		nanos = 0
	}
	base := nanos / 1_000_000_000
	hour := base / 3600
	minute := (base % 3600) / 60
	second := base % 60

	var b strings.Builder
	fmt.Fprintf(&b, "%02d:%02d:%02d", hour, minute, second)
	b.WriteByte(sep)
	frac := nanos % 1_000_000_000
	for i := 9; i > fracDigits; i-- {
		frac /= 10
	}
	fmt.Fprintf(&b, "%0*d", fracDigits, frac)
	return b.String()
}
