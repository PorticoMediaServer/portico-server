package app

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"math"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	quickConnectAlphabet       = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	quickConnectCodeLength     = 8
	quickConnectCodeAttempts   = 10
	quickConnectLimiterMaxKeys = 4096
)

var (
	errQuickConnectEntropyUnavailable = errors.New("quick connect entropy unavailable")
	errQuickConnectCodeSpaceBusy      = errors.New("quick connect code space is temporarily unavailable")
	errQuickConnectNotFound           = errors.New("quick connect request was not found")
	errQuickConnectNotApproved        = errors.New("quick connect request has not been approved")
	errQuickConnectExpired            = errors.New("quick connect request has expired")
	errQuickConnectAlreadyUsed        = errors.New("quick connect request has already been used")
)

type quickConnectRateKind string

const (
	quickConnectRateStart    quickConnectRateKind = "start"
	quickConnectRateStatus   quickConnectRateKind = "status"
	quickConnectRateExchange quickConnectRateKind = "exchange"
)

type quickConnectRatePolicy struct {
	limit  int
	window time.Duration
	code   string
	detail string
}

func quickConnectPolicy(kind quickConnectRateKind) quickConnectRatePolicy {
	switch kind {
	case quickConnectRateStatus:
		return quickConnectRatePolicy{limit: 120, window: time.Minute, code: "quick_connect_status_rate_limited", detail: "Quick Connect status is being checked too frequently."}
	case quickConnectRateExchange:
		return quickConnectRatePolicy{limit: 10, window: time.Minute, code: "quick_connect_exchange_rate_limited", detail: "Too many Quick Connect exchange attempts."}
	default:
		return quickConnectRatePolicy{limit: 6, window: time.Minute, code: "quick_connect_start_rate_limited", detail: "Too many Quick Connect requests have been started."}
	}
}

type boundedWindowRateLimiter struct {
	mu        sync.Mutex
	attempts  map[string][]time.Time
	lastPrune time.Time
	maxKeys   int
}

func (l *boundedWindowRateLimiter) allow(key string, policy quickConnectRatePolicy, now time.Time) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.attempts == nil {
		l.attempts = map[string][]time.Time{}
	}
	maxKeys := l.maxKeys
	if maxKeys <= 0 {
		maxKeys = quickConnectLimiterMaxKeys
	}
	if l.lastPrune.IsZero() || now.Sub(l.lastPrune) >= time.Minute || len(l.attempts) >= maxKeys {
		l.prune(now.Add(-2 * time.Minute))
		l.lastPrune = now
	}
	key = strings.TrimSpace(key)
	if key == "" {
		key = "unknown"
	}
	cutoff := now.Add(-policy.window)
	recent := l.attempts[key][:0]
	for _, attempt := range l.attempts[key] {
		if attempt.After(cutoff) {
			recent = append(recent, attempt)
		}
	}
	if len(recent) == 0 {
		delete(l.attempts, key)
	} else {
		l.attempts[key] = recent
	}
	if len(recent) >= policy.limit {
		retry := int(math.Ceil(recent[0].Add(policy.window).Sub(now).Seconds()))
		if retry < 1 {
			retry = 1
		}
		return false, retry
	}
	if _, exists := l.attempts[key]; !exists && len(l.attempts) >= maxKeys {
		l.evictOldest()
	}
	l.attempts[key] = append(recent, now)
	return true, 0
}

func (l *boundedWindowRateLimiter) prune(cutoff time.Time) {
	for key, attempts := range l.attempts {
		if len(attempts) == 0 || !attempts[len(attempts)-1].After(cutoff) {
			delete(l.attempts, key)
		}
	}
}

func (l *boundedWindowRateLimiter) evictOldest() {
	oldestKey := ""
	var oldest time.Time
	for key, attempts := range l.attempts {
		if len(attempts) == 0 {
			delete(l.attempts, key)
			continue
		}
		candidate := attempts[len(attempts)-1]
		if oldestKey == "" || candidate.Before(oldest) {
			oldestKey, oldest = key, candidate
		}
	}
	if oldestKey != "" {
		delete(l.attempts, oldestKey)
	}
}

func (s *Server) allowQuickConnectPublicRequest(w http.ResponseWriter, r *http.Request, kind quickConnectRateKind) bool {
	policy := quickConnectPolicy(kind)
	key := string(kind) + "\x00" + clientIPFromRequest(r)
	allowed, retryAfter := s.quickConnectLimiter.allow(key, policy, time.Now().UTC())
	if allowed {
		return true
	}
	w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
	writeError(w, http.StatusTooManyRequests, policy.code, policy.detail)
	return false
}

func quickConnectNoStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
}

func (s *Server) quickConnectEntropyReader() io.Reader {
	if s != nil && s.quickConnectEntropy != nil {
		return s.quickConnectEntropy
	}
	return rand.Reader
}

func randomQuickConnectSecret(reader io.Reader) (string, error) {
	bytes := make([]byte, 32)
	if _, err := io.ReadFull(reader, bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func randomQuickConnectRequestID(reader io.Reader) (string, error) {
	bytes := make([]byte, 12)
	if _, err := io.ReadFull(reader, bytes); err != nil {
		return "", err
	}
	return "qcx_" + base64.RawURLEncoding.EncodeToString(bytes), nil
}

func randomQuickConnectCode(reader io.Reader) (string, error) {
	if reader == nil {
		return "", errQuickConnectEntropyUnavailable
	}
	code := make([]byte, quickConnectCodeLength)
	limit := big.NewInt(int64(len(quickConnectAlphabet)))
	for index := range code {
		value, err := rand.Int(reader, limit)
		if err != nil {
			return "", err
		}
		code[index] = quickConnectAlphabet[value.Int64()]
	}
	return formatQuickConnectCode(string(code)), nil
}

func normalizeQuickConnectCode(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	code := make([]byte, 0, quickConnectCodeLength)
	dashes := 0
	for index := 0; index < len(value); index++ {
		character := value[index]
		switch character {
		case ' ':
			continue
		case '-':
			dashes++
			if dashes > 1 {
				return ""
			}
			continue
		}
		if !strings.ContainsRune(quickConnectAlphabet, rune(character)) {
			return ""
		}
		code = append(code, character)
	}
	if len(code) != quickConnectCodeLength {
		return ""
	}
	return string(code)
}

func formatQuickConnectCode(code string) string {
	if len(code) != quickConnectCodeLength {
		return code
	}
	return code[:4] + "-" + code[4:]
}

func quickConnectCodeForLookup(value string) string {
	if normalized := normalizeQuickConnectCode(value); normalized != "" {
		return formatQuickConnectCode(normalized)
	}
	return ""
}

func quickConnectProtocolForCode(code string) int {
	return 1
}

func isQuickConnectCodeConflict(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint failed") && strings.Contains(message, "quick_connect_requests.code")
}
