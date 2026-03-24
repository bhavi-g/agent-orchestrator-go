package repair

import (
	"math"
	"time"
)

// RetryConfig defines retry behavior
type RetryConfig struct {
	// MaxAttempts is the maximum number of retry attempts
	MaxAttempts int
	// InitialDelay is the delay before the first retry
	InitialDelay time.Duration
	// MaxDelay is the maximum delay between retries
	MaxDelay time.Duration
	// BackoffMultiplier is the multiplier for exponential backoff
	BackoffMultiplier float64
	// EnableJitter adds randomness to delays to avoid thundering herd
	EnableJitter bool
}

// DefaultRetryConfig returns sensible defaults
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:       3,
		InitialDelay:      100 * time.Millisecond,
		MaxDelay:          10 * time.Second,
		BackoffMultiplier: 2.0,
		EnableJitter:      true,
	}
}

// CalculateDelay computes the delay for a given attempt number (1-based)
func (c RetryConfig) CalculateDelay(attempt int) time.Duration {
	if attempt <= 1 {
		return 0 // No delay for first attempt
	}

	// Exponential backoff: initialDelay * (multiplier ^ (attempt - 2))
	retryNumber := attempt - 1 // 0-based retry count
	delay := float64(c.InitialDelay) * math.Pow(c.BackoffMultiplier, float64(retryNumber-1))

	// Cap at max delay
	if delay > float64(c.MaxDelay) {
		delay = float64(c.MaxDelay)
	}

	duration := time.Duration(delay)

	// Add jitter if enabled (±25%)
	if c.EnableJitter && retryNumber > 0 {
		jitterRange := float64(duration) * 0.25
		jitter := (time.Duration(retryNumber*13) % time.Duration(jitterRange*2)) - time.Duration(jitterRange)
		duration += jitter
		if duration < 0 {
			duration = 0
		}
	}

	return duration
}

// ShouldRetry checks if we should retry given the attempt number
func (c RetryConfig) ShouldRetry(attempt int) bool {
	return attempt < c.MaxAttempts
}
