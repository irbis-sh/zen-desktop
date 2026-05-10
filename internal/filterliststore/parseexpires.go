package filterliststore

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	// expiresRegex matches lines like "! Expires: 4 days", supporting formats such as: "4 days", "12 hours", "5d", and "18h".
	expiresRegex  = regexp.MustCompile(`(?i)^! Expires:\s*(\d+)\s*(days?|hours?|d|h)?`)
	errNotExpires = errors.New("not an expires line")
)

// parseExpires parses the line and returns the duration if it matches the expected format.
func parseExpires(line []byte) (time.Duration, error) {
	matches := expiresRegex.FindSubmatch(line)
	if matches == nil {
		return time.Duration(0), errNotExpires
	}

	amount, err := strconv.Atoi(string(matches[1]))
	if err != nil {
		return time.Duration(0), fmt.Errorf("invalid amount: %v", err)
	}

	unit := "days"
	if len(matches) >= 3 {
		unit = strings.ToLower(strings.TrimSpace(string(matches[2])))
	}

	switch unit {
	case "day", "days", "d":
		return time.Duration(amount) * 24 * time.Hour, nil
	case "hour", "hours", "h":
		return time.Duration(amount) * time.Hour, nil
	default:
		return time.Duration(0), errors.New("invalid time unit")
	}
}
