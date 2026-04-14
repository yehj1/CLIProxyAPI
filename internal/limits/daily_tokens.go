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

// Snapshot returns a deep copy of all daily token totals.
func (l *DailyTokenLimiter) Snapshot() map[string]map[string]int64 {
	if l == nil {
		return map[string]map[string]int64{}
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	return cloneTotals(l.totals)
}

// ReplaceTotals replaces all daily token totals with the provided snapshot.
func (l *DailyTokenLimiter) ReplaceTotals(totals map[string]map[string]int64) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.totals = cloneTotals(totals)
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
	return dayKeyForLocation(at, loc)
}

func (l *DailyTokenLimiter) pruneOldDays(perKey map[string]int64, currentDay string) {
	if len(perKey) == 0 {
		return
	}
	for k := range perKey {
		if k == currentDay {
			continue
		}
		pruneOldDaysForLocation(perKey, currentDay, l.location())
		break
	}
}

func dayKeyForLocation(at time.Time, loc *time.Location) string {
	if at.IsZero() {
		at = time.Now()
	}
	if loc == nil {
		loc = time.Local
	}
	return at.In(loc).Format("2006-01-02")
}

func pruneOldDaysForLocation(perKey map[string]int64, currentDay string, loc *time.Location) {
	if len(perKey) == 0 {
		return
	}
	if loc == nil {
		loc = time.Local
	}
	for k := range perKey {
		if k == currentDay {
			continue
		}
		if day, err := time.ParseInLocation("2006-01-02", k, loc); err == nil {
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

func cloneTotals(src map[string]map[string]int64) map[string]map[string]int64 {
	if len(src) == 0 {
		return map[string]map[string]int64{}
	}
	dst := make(map[string]map[string]int64, len(src))
	for key, perDay := range src {
		if len(perDay) == 0 {
			dst[key] = map[string]int64{}
			continue
		}
		copied := make(map[string]int64, len(perDay))
		for day, value := range perDay {
			copied[day] = value
		}
		dst[key] = copied
	}
	return dst
}
