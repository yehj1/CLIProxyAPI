package limits

import (
	"context"
	"time"

	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

type dailyTokenUsagePlugin struct {
	limiter *DailyTokenLimiter
}

func NewDailyTokenUsagePlugin(limiter *DailyTokenLimiter) coreusage.Plugin {
	if limiter == nil {
		limiter = GetDailyTokenLimiter()
	}
	return &dailyTokenUsagePlugin{limiter: limiter}
}

func (p *dailyTokenUsagePlugin) HandleUsage(_ context.Context, record coreusage.Record) {
	if p == nil || p.limiter == nil {
		return
	}
	apiKey := record.APIKey
	if apiKey == "" {
		return
	}
	tokens := record.Detail.TotalTokens
	if tokens <= 0 {
		return
	}
	ts := record.RequestedAt
	if ts.IsZero() {
		ts = time.Now()
	}
	p.limiter.AddTokens(apiKey, tokens, ts)

	rate := CreditPerMillionTokens()
	if rate > 0 {
		credits := tokensToCredits(tokens, rate)
		if credits > 0 {
			GetDailyCreditLimiter().AddCredits(apiKey, credits, ts)
		}
	}
}

func init() {
	coreusage.RegisterPlugin(NewDailyTokenUsagePlugin(GetDailyTokenLimiter()))
}

func tokensToCredits(tokens, rate int64) int64 {
	if tokens <= 0 || rate <= 0 {
		return 0
	}
	unit := CreditUnitTokens()
	if unit <= 0 {
		return 0
	}
	// ceil(tokens / unit) * rate
	units := (tokens + unit - 1) / unit
	return units * rate
}
