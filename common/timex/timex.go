package timex

import (
	"fmt"
	"strconv"
	"time"
)

// ParseDuration parses a duration string in the standard Go duration format
// (e.g. "300ms", "1.5h", "2h45m"), extended with a "d" (day) unit, where
// 1d = 24h; for example "1d12h30m" is 36h30m. The maximum input string
// length is 1024.
func ParseDuration(s string) (time.Duration, error) {
	if len(s) > 1024 {
		return 0, fmt.Errorf("parsing duration: input string too long")
	}
	var inNumber bool
	var numStart int
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == 'd' {
			daysStr := s[numStart:i]
			days, err := strconv.ParseFloat(daysStr, 64)
			if err != nil {
				return 0, err
			}
			hours := days * 24.0
			hoursStr := strconv.FormatFloat(hours, 'f', -1, 64)
			s = s[:numStart] + hoursStr + "h" + s[i+1:]
			i--
			continue
		}
		if !inNumber {
			numStart = i
		}
		inNumber = (ch >= '0' && ch <= '9') || ch == '.' || ch == '-' || ch == '+'
	}
	return time.ParseDuration(s)
}
