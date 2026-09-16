package provider

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Kind string

const (
	KindUnauthorized Kind = "unauthorized"
	KindNotFound     Kind = "not_found"
	KindRateLimit    Kind = "rate_limit"
	KindUnsupported  Kind = "unsupported"
	KindUpstream     Kind = "upstream"
)

type Error struct {
	Kind        Kind
	RetryAfter  time.Duration
	ProviderMsg string
}

func (e *Error) Error() string {
	msg := fmt.Sprintf("provider error: %s", e.Kind)
	if e.ProviderMsg != "" {
		msg += ": " + e.ProviderMsg
	}
	if e.RetryAfter > 0 {
		msg += fmt.Sprintf(" (retry after %s)", e.RetryAfter)
	}
	return msg
}

func MapHTTP(status int, body []byte, headers map[string][]string) *Error {
	kind := kindFromStatus(status)
	msg := strings.TrimSpace(scrubTokens(string(body)))

	err := &Error{Kind: kind, ProviderMsg: msg}

	if kind == KindRateLimit {
		if ra, ok := parseRetryAfter(headers); ok {
			err.RetryAfter = ra
		}
	}

	return err
}

func kindFromStatus(status int) Kind {
	switch status {
	case 401:
		return KindUnauthorized
	case 404:
		return KindNotFound
	case 429:
		return KindRateLimit
	case 501:
		return KindUnsupported
	default:
		return KindUpstream
	}
}

func parseRetryAfter(headers map[string][]string) (time.Duration, bool) {
	vals := headers["Retry-After"]
	if len(vals) == 0 {
		vals = headers["retry-after"]
	}
	if len(vals) == 0 {
		return 0, false
	}
	v := strings.TrimSpace(vals[0])
	if secs, err := strconv.Atoi(v); err == nil {
		return time.Duration(secs) * time.Second, true
	}
	if t, err := time.Parse(time.RFC1123, v); err == nil {
		d := time.Until(t)
		if d > 0 {
			return d, true
		}
		return 0, false
	}
	return 0, false
}

func scrubTokens(s string) string {
	result := s
	patterns := []struct {
		search  string
		replace string
	}{
		{`ghp_[A-Za-z0-9]{36}`, "ghp_***"},
		{`gho_[A-Za-z0-9]{36}`, "gho_***"},
		{`ghu_[A-Za-z0-9]{36}`, "ghu_***"},
		{`ghs_[A-Za-z0-9]{36}`, "ghs_***"},
		{`ghr_[A-Za-z0-9]{36}`, "ghr_***"},
		{`Bearer [A-Za-z0-9\-_.]+`, "Bearer ***"},
		{`Authorization: [^\r\n]+`, "Authorization: ***"},
	}
	for _, p := range patterns {
		result = strings.ReplaceAll(result, p.search, p.replace)
	}
	return result
}