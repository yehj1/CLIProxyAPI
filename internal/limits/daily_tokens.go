package limits

import (
	"sync"
	"sync/atomic"
	"time"
)

// DailyTokenLimiter tracks per-API-key token usage by day.
// It is used to enforce daily token limits on incoming requests.
type DailyTokenLimiter struct {
	mu     sync.RWMutex
	totals map[string]map[string]int64
	tz     atomic.Value
}

func NewDailyTokenLimiter() *DailyTokenLimiter {
	limiter := &DailyTokenLimiter{
		totals: make(map[string]map[string]int64),
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.Local
	}
	limiter.tz.Store(loc)
	return limiter
}

func (l *DailyTokenLimiter) TokensUsed(apiKey string, at time.Time) int64 {
	if l == nil || apiKey == "" {
		return 0
	}
	dayKey := l.dayKey(at)
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

func (l *DailyTokenLimiter) AddTokens(apiKey string, tokens int64, at time.Time) {
	if l == nil || apiKey == "" || tokens <= 0 {
		return
	}
	dayKey := l.dayKey(at)
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
	perKey[dayKey] += tokens
	if len(perKey) > 32 {
		l.pruneOldDays(perKey, dayKey)
	}
}

func (l *DailyTokenLimiter) dayKey(at time.Time) string {
	if at.IsZero() {
		at = time.Now()
	}
	loc := l.location()
	return at.In(loc).Format("2006-01-02")
}

func (l *DailyTokenLimiter) pruneOldDays(perKey map[string]int64, currentDay string) {
	if len(perKey) == 0 {
		return
	}
	for k := range perKey {
		if k == currentDay {
			continue
		}
		if day, err := time.ParseInLocation("2006-01-02", k, l.location()); err == nil {
			if time.Since(day) > 45*24*time.Hour {
				delete(perKey, k)
			}
		}
	}
}

var defaultDailyLimiter = NewDailyTokenLimiter()

func GetDailyTokenLimiter() *DailyTokenLimiter { return defaultDailyLimiter }

func DailyTokenLimitLocation() *time.Location {
	return defaultDailyLimiter.location()
}

func (l *DailyTokenLimiter) SetLocation(loc *time.Location) {
	if l == nil {
		return
	}
	if loc == nil {
		loc = time.Local
	}
	l.tz.Store(loc)
}

func (l *DailyTokenLimiter) location() *time.Location {
	if l == nil {
		return time.Local
	}
	if v := l.tz.Load(); v != nil {
		if loc, ok := v.(*time.Location); ok && loc != nil {
			return loc
		}
	}
	return time.Local
}
