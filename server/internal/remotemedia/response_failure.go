package remotemedia

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ResponseFailure describes only the response Portico actually observed. On a
// serve endpoint this is rclone's response, not proof of the provider's status
// or requested delay. It deliberately contains no raw headers, URL or log text.
type ResponseFailure struct {
	Status  int
	Kind    string
	RetryAt time.Time
}

func (e *ResponseFailure) Error() string { return "remote source request could not complete" }

func responseFailure(resp *http.Response, now time.Time) error {
	kind := "unavailable"
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrDenied
	case http.StatusTooManyRequests:
		kind = "throttled"
	case http.StatusInsufficientStorage:
		kind = "quota"
	case http.StatusServiceUnavailable:
	default:
		return ErrUnavailable
	}
	e := &ResponseFailure{Status: resp.StatusCode, Kind: kind}
	if resp.StatusCode == 429 || resp.StatusCode == 503 || resp.StatusCode == 507 {
		e.RetryAt = retryAt(resp.Header.Get("Retry-After"), now)
	}
	return e
}

func retryAt(raw string, now time.Time) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	digits := true
	for _, c := range raw {
		digits = digits && c >= '0' && c <= '9'
	}
	if digits {
		seconds, err := strconv.ParseInt(raw, 10, 64)
		// Reject overflow rather than inventing a shorter requested wait.
		if err != nil || seconds > int64((1<<63-1)/time.Second) {
			return time.Time{}
		}
		return now.Add(time.Duration(seconds) * time.Second)
	}
	if deadline, err := http.ParseTime(raw); err == nil {
		return deadline
	}
	return time.Time{}
}
