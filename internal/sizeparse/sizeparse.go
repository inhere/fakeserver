package sizeparse

import (
	"fmt"
	"strings"
)

// ParseByteSize converts "10MiB", "16B", "1KiB" etc into a byte count.
// Supports decimal (B/KB/MB/GB/TB) and binary (KiB/MiB/GiB/TiB) units.
func ParseByteSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty size")
	}
	type unit struct {
		suffix string
		mult   int64
	}
	units := []unit{
		{"TiB", 1 << 40},
		{"GiB", 1 << 30},
		{"MiB", 1 << 20},
		{"KiB", 1 << 10},
		{"TB", 1_000_000_000_000},
		{"GB", 1_000_000_000},
		{"MB", 1_000_000},
		{"KB", 1_000},
		{"B", 1},
	}
	for _, u := range units {
		if strings.HasSuffix(s, u.suffix) {
			numStr := strings.TrimSpace(strings.TrimSuffix(s, u.suffix))
			for _, ch := range numStr {
				if ch < '0' || ch > '9' {
					return 0, fmt.Errorf("byte size %q: non-numeric prefix %q before suffix %q", s, numStr, u.suffix)
				}
			}
			if numStr == "" {
				return 0, fmt.Errorf("byte size %q: missing numeric value before suffix %q", s, u.suffix)
			}
			var n int64
			_, err := fmt.Sscanf(numStr, "%d", &n)
			if err != nil {
				return 0, fmt.Errorf("byte size %q: %w", s, err)
			}
			return n * u.mult, nil
		}
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("byte size %q: invalid characters (only uppercase B/KB/MB/GB/TB and KiB/MiB/GiB/TiB suffixes recognized)", s)
		}
	}
	var n int64
	_, err := fmt.Sscanf(s, "%d", &n)
	if err != nil {
		return 0, fmt.Errorf("byte size %q: %w", s, err)
	}
	return n, nil
}
