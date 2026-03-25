package retry

import (
	"math"
	"math/rand"
	"time"
)

// BackoffType defines the backoff algorithm to use.
type BackoffType string

const (
	// BackoffNone means no delay between retries.
	BackoffNone BackoffType = "none"
	// BackoffConstant uses the same delay for every retry.
	BackoffConstant BackoffType = "constant"
	// BackoffLinear increases delay linearly: initialDelay * attempt.
	BackoffLinear BackoffType = "linear"
	// BackoffExponential doubles delay each retry: initialDelay * 2^(attempt-1).
	BackoffExponential BackoffType = "exponential"
)

// Policy defines the complete retry behaviour for a unit of work.
type Policy struct {
	// MaxAttempts is the total number of attempts (first try + retries).
	// 1 means no retries. 0 falls back to DefaultPolicy().MaxAttempts.
	MaxAttempts int

	// Backoff selects the algorithm. Default is BackoffExponential.
	Backoff BackoffType

	// InitialDelay is the base delay used by the backoff algorithm.
	InitialDelay time.Duration

	// MaxDelay caps the computed delay.
	MaxDelay time.Duration

	// Multiplier is used by exponential backoff (default 2.0).
	Multiplier float64

	// Jitter when true adds ±25 % randomness to each delay.
	Jitter bool

	// RetryableErrors limits retries to failures whose message contains
	// one of these substrings. An empty slice means "retry any error".
	RetryableErrors []string
}

// DefaultPolicy returns production-ready defaults.
func DefaultPolicy() Policy {
	return Policy{
		MaxAttempts:   3,
		Backoff:       BackoffExponential,
		InitialDelay:  100 * time.Millisecond,
		MaxDelay:      10 * time.Second,
		Multiplier:    2.0,
		Jitter:        true,
		RetryableErrors: nil,
	}
}

// NoRetryPolicy returns a policy that never retries.
func NoRetryPolicy() Policy {
	return Policy{
		MaxAttempts: 1,
		Backoff:     BackoffNone,
	}
}

// Merge returns a copy of p where zero-valued fields are filled from
// fallback. This lets a per-step policy override only the fields it cares
// about while inheriting the rest from the global default.
func (p Policy) Merge(fallback Policy) Policy {
	out := p
	if out.MaxAttempts == 0 {
		out.MaxAttempts = fallback.MaxAttempts
	}
	if out.Backoff == "" {
		out.Backoff = fallback.Backoff
	}
	if out.InitialDelay == 0 {
		out.InitialDelay = fallback.InitialDelay
	}
	if out.MaxDelay == 0 {
		out.MaxDelay = fallback.MaxDelay
	}
	if out.Multiplier == 0 {
		out.Multiplier = fallback.Multiplier
	}
	return out
}

// ShouldRetry reports whether the given attempt number (1-based) is below
// the maximum.
func (p Policy) ShouldRetry(attempt int) bool {
	max := p.MaxAttempts
	if max == 0 {
		max = DefaultPolicy().MaxAttempts
	}
	return attempt < max
}

// Delay computes the wait duration before the given attempt (1-based).
// Attempt 1 always returns 0 (no delay for the first try).
func (p Policy) Delay(attempt int) time.Duration {
	if attempt <= 1 {
		return 0
	}

	retryNum := attempt - 1 // 0-based retry index

	var raw float64
	switch p.Backoff {
	case BackoffNone:
		return 0
	case BackoffConstant:
		raw = float64(p.InitialDelay)
	case BackoffLinear:
		raw = float64(p.InitialDelay) * float64(retryNum)
	case BackoffExponential:
		mult := p.Multiplier
		if mult == 0 {
			mult = 2.0
		}
		raw = float64(p.InitialDelay) * math.Pow(mult, float64(retryNum-1))
	default:
		// Treat unknown as exponential
		mult := p.Multiplier
		if mult == 0 {
			mult = 2.0
		}
		raw = float64(p.InitialDelay) * math.Pow(mult, float64(retryNum-1))
	}

	// Cap
	if p.MaxDelay > 0 && raw > float64(p.MaxDelay) {
		raw = float64(p.MaxDelay)
	}

	d := time.Duration(raw)

	// Jitter ±25 %
	if p.Jitter && d > 0 {
		jitterFraction := 0.25
		jitterRange := float64(d) * jitterFraction
		d += time.Duration(rand.Float64()*2*jitterRange - jitterRange)
		if d < 0 {
			d = 0
		}
	}

	return d
}
