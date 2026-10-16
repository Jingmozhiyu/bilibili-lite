package biz

import (
	"context"
	"strconv"
	"time"

	"github.com/go-kratos/kratos/v3/errors"
)

// PurchaseGuard coordinates shared account admission and bounds local transactions.
type PurchaseGuard interface {
	ObserveResult(string)
	Check(context.Context, uint64, string) error
	Cooldown(context.Context, uint64, string, time.Duration) error
	Acquire() (func(), bool)
}

// HTTP code stays a valid HTTP status; BW-style codes are business metadata.
func PurchaseError(httpCode int, code, message string, retryMS int64) *errors.Error {
	return errors.New(httpCode, "MALL_PURCHASE_"+code, message).WithMetadata(map[string]string{
		"business_code":  code,
		"retry_after_ms": strconv.FormatInt(retryMS, 10),
	})
}
