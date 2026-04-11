package limits

import "sync/atomic"

const creditUnitTokens int64 = 1_000_000

var creditPerMillionTokens int64

func SetCreditPerMillionTokens(value int64) {
	if value < 0 {
		value = 0
	}
	atomic.StoreInt64(&creditPerMillionTokens, value)
}

func CreditPerMillionTokens() int64 {
	return atomic.LoadInt64(&creditPerMillionTokens)
}

func CreditUnitTokens() int64 {
	return creditUnitTokens
}
