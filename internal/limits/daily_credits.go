package limits

import (
	"sync"
	"time"
)

// DailyCreditLimiter tracks per-API-key credit usage by day.
type DailyCreditLimiter struct {
	mu     sync.RWMutex
	totals map[string]map[string]int64
}

func NewDailyCreditLimiter() *DailyCreditLimiter {
	return &DailyCreditLimiter{
		totals: make(map[string]map[string]int64),
	}
}

func (l *DailyCreditLimiter) CreditsUsed(apiKey string, at time.Time) int64 {
	if l == nil || apiKey == "" {
		return 0
	}
	dayKey := dayKeyForLocation(at, DailyTokenLimitLocation())
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.totals == nil {
		return 0
	}
	if perKey, ok := l.totals[apiKey]; ok {
		return perKey[dayKey]
	}
	return 0
}

func (l *DailyCreditLimiter) AddCredits(apiKey string, credits int64, at time.Time) {
	if l == nil || apiKey == "" || credits <= 0 {
		return
	}
	dayKey := dayKeyForLocation(at, DailyTokenLimitLocation())
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.totals == nil {
		l.totals = make(map[string]map[string]int64)
	}
	perKey := l.totals[apiKey]
	if perKey == nil {
		perKey = make(map[string]int64)
		l.totals[apiKey] = perKey
	}
	perKey[dayKey] += credits
	if len(perKey) > 32 {
		pruneOldDaysForLocation(perKey, dayKey, DailyTokenLimitLocation())
	}
}

var defaultDailyCreditLimiter = NewDailyCreditLimiter()

func GetDailyCreditLimiter() *DailyCreditLimiter { return defaultDailyCreditLimiter }
