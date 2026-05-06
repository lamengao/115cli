package open115

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

func parseInfoSize(value string) (int64, error) {
	s := strings.TrimSpace(value)
	if s == "" {
		return 0, nil
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n, nil
	}
	units := []struct {
		suffix     string
		multiplier float64
	}{
		{"PB", 1024 * 1024 * 1024 * 1024 * 1024},
		{"TB", 1024 * 1024 * 1024 * 1024},
		{"GB", 1024 * 1024 * 1024},
		{"MB", 1024 * 1024},
		{"KB", 1024},
		{"B", 1},
	}
	upper := strings.ToUpper(s)
	for _, unit := range units {
		if !strings.HasSuffix(upper, unit.suffix) {
			continue
		}
		number := strings.TrimSpace(s[:len(s)-len(unit.suffix)])
		if number == "" {
			return 0, fmt.Errorf("invalid size %q", value)
		}
		n, err := strconv.ParseFloat(number, 64)
		if err != nil {
			return 0, err
		}
		if n < 0 {
			return 0, fmt.Errorf("invalid negative size %q", value)
		}
		return int64(math.Round(n * unit.multiplier)), nil
	}
	return 0, fmt.Errorf("invalid size %q", value)
}
