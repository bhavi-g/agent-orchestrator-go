package retry

import (
	"testing"
	"time"
)

func TestDefaultPolicy(t *testing.T) {
	p := DefaultPolicy()

	if p.MaxAttempts != 3 {
		t.Errorf("expected MaxAttempts 3, got %d", p.MaxAttempts)
	}
	if p.Backoff != BackoffExponential {
		t.Errorf("expected BackoffExponential, got %s", p.Backoff)
	}
	if p.InitialDelay != 100*time.Millisecond {
		t.Errorf("expected 100ms InitialDelay, got %v", p.InitialDelay)
	}
	if p.MaxDelay != 10*time.Second {
		t.Errorf("expected 10s MaxDelay, got %v", p.MaxDelay)
	}
	if p.Multiplier != 2.0 {
		t.Errorf("expected 2.0 Multiplier, got %f", p.Multiplier)
	}
	if !p.Jitter {
		t.Error("expected Jitter to be true")
	}
}

func TestNoRetryPolicy(t *testing.T) {
	p := NoRetryPolicy()

	if p.MaxAttempts != 1 {
		t.Errorf("expected MaxAttempts 1, got %d", p.MaxAttempts)
	}
	if p.ShouldRetry(1) {
		t.Error("NoRetryPolicy should not allow retry after attempt 1")
	}
}

func TestShouldRetry(t *testing.T) {
	p := Policy{MaxAttempts: 3}

	tests := []struct {
		attempt int
		want    bool
	}{
		{1, true},
		{2, true},
		{3, false},
		{4, false},
	}

	for _, tt := range tests {
		got := p.ShouldRetry(tt.attempt)
		if got != tt.want {
			t.Errorf("ShouldRetry(%d) = %v, want %v", tt.attempt, got, tt.want)
		}
	}
}

func TestShouldRetry_ZeroDefaultsToThree(t *testing.T) {
	p := Policy{MaxAttempts: 0}

	if !p.ShouldRetry(1) {
		t.Error("zero MaxAttempts should default to 3; attempt 1 should be retryable")
	}
	if !p.ShouldRetry(2) {
		t.Error("zero MaxAttempts should default to 3; attempt 2 should be retryable")
	}
	if p.ShouldRetry(3) {
		t.Error("zero MaxAttempts should default to 3; attempt 3 should NOT be retryable")
	}
}

func TestDelay_FirstAttemptIsZero(t *testing.T) {
	p := DefaultPolicy()
	p.Jitter = false

	d := p.Delay(1)
	if d != 0 {
		t.Errorf("first attempt should have 0 delay, got %v", d)
	}
}

func TestDelay_BackoffNone(t *testing.T) {
	p := Policy{Backoff: BackoffNone, InitialDelay: 100 * time.Millisecond}

	for attempt := 1; attempt <= 5; attempt++ {
		d := p.Delay(attempt)
		if d != 0 {
			t.Errorf("BackoffNone attempt %d: expected 0, got %v", attempt, d)
		}
	}
}

func TestDelay_BackoffConstant(t *testing.T) {
	p := Policy{Backoff: BackoffConstant, InitialDelay: 200 * time.Millisecond}

	// Attempt 1 → 0, attempt 2+ → 200ms
	if d := p.Delay(1); d != 0 {
		t.Errorf("attempt 1: expected 0, got %v", d)
	}
	if d := p.Delay(2); d != 200*time.Millisecond {
		t.Errorf("attempt 2: expected 200ms, got %v", d)
	}
	if d := p.Delay(5); d != 200*time.Millisecond {
		t.Errorf("attempt 5: expected 200ms, got %v", d)
	}
}

func TestDelay_BackoffLinear(t *testing.T) {
	p := Policy{Backoff: BackoffLinear, InitialDelay: 100 * time.Millisecond}

	// retryNum = attempt - 1, delay = initialDelay * retryNum
	expected := map[int]time.Duration{
		1: 0,
		2: 100 * time.Millisecond,  // retryNum=1
		3: 200 * time.Millisecond,  // retryNum=2
		4: 300 * time.Millisecond,  // retryNum=3
	}

	for attempt, want := range expected {
		got := p.Delay(attempt)
		if got != want {
			t.Errorf("attempt %d: expected %v, got %v", attempt, want, got)
		}
	}
}

func TestDelay_BackoffExponential(t *testing.T) {
	p := Policy{
		Backoff:      BackoffExponential,
		InitialDelay: 100 * time.Millisecond,
		Multiplier:   2.0,
	}

	// retryNum = attempt-1, delay = 100ms * 2^(retryNum-1)
	expected := map[int]time.Duration{
		1: 0,
		2: 100 * time.Millisecond,  // 100ms * 2^0
		3: 200 * time.Millisecond,  // 100ms * 2^1
		4: 400 * time.Millisecond,  // 100ms * 2^2
	}

	for attempt, want := range expected {
		got := p.Delay(attempt)
		if got != want {
			t.Errorf("attempt %d: expected %v, got %v", attempt, want, got)
		}
	}
}

func TestDelay_MaxDelayCap(t *testing.T) {
	p := Policy{
		Backoff:      BackoffExponential,
		InitialDelay: 1 * time.Second,
		MaxDelay:     5 * time.Second,
		Multiplier:   10.0,
	}

	// Attempt 3: 1s * 10^1 = 10s, capped at 5s
	got := p.Delay(3)
	if got != 5*time.Second {
		t.Errorf("expected delay capped at 5s, got %v", got)
	}
}

func TestDelay_JitterAddsVariance(t *testing.T) {
	p := Policy{
		MaxAttempts:  5,
		Backoff:      BackoffExponential,
		InitialDelay: 100 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       true,
	}

	// Run several times and check that at least one differs from base
	base := 100 * time.Millisecond // expected for attempt 2 without jitter
	allSame := true
	for i := 0; i < 20; i++ {
		d := p.Delay(2)
		if d != base {
			allSame = false
		}
		// Jitter should keep it within ±25%
		low := time.Duration(float64(base) * 0.75)
		high := time.Duration(float64(base) * 1.25)
		if d < low || d > high {
			t.Errorf("jittered delay %v outside ±25%% of %v", d, base)
		}
	}
	if allSame {
		t.Error("expected jitter to produce at least some variance over 20 trials")
	}
}

func TestMerge_OverrideFields(t *testing.T) {
	fallback := DefaultPolicy()

	override := Policy{
		MaxAttempts: 5,
		// All other fields left as zero -> inherited from fallback
	}

	merged := override.Merge(fallback)

	if merged.MaxAttempts != 5 {
		t.Errorf("expected MaxAttempts 5, got %d", merged.MaxAttempts)
	}
	if merged.Backoff != BackoffExponential {
		t.Errorf("expected inherited Backoff, got %s", merged.Backoff)
	}
	if merged.InitialDelay != 100*time.Millisecond {
		t.Errorf("expected inherited InitialDelay, got %v", merged.InitialDelay)
	}
	if merged.MaxDelay != 10*time.Second {
		t.Errorf("expected inherited MaxDelay, got %v", merged.MaxDelay)
	}
	if merged.Multiplier != 2.0 {
		t.Errorf("expected inherited Multiplier, got %f", merged.Multiplier)
	}
}

func TestMerge_FullOverride(t *testing.T) {
	fallback := DefaultPolicy()

	override := Policy{
		MaxAttempts:  10,
		Backoff:      BackoffConstant,
		InitialDelay: 500 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   1.0,
	}

	merged := override.Merge(fallback)

	if merged.MaxAttempts != 10 {
		t.Errorf("expected 10, got %d", merged.MaxAttempts)
	}
	if merged.Backoff != BackoffConstant {
		t.Errorf("expected BackoffConstant, got %s", merged.Backoff)
	}
	if merged.InitialDelay != 500*time.Millisecond {
		t.Errorf("expected 500ms, got %v", merged.InitialDelay)
	}
	if merged.MaxDelay != 1*time.Second {
		t.Errorf("expected 1s, got %v", merged.MaxDelay)
	}
	if merged.Multiplier != 1.0 {
		t.Errorf("expected 1.0, got %f", merged.Multiplier)
	}
}

func TestMerge_EmptyInheritsAll(t *testing.T) {
	fallback := DefaultPolicy()
	merged := Policy{}.Merge(fallback)

	if merged.MaxAttempts != fallback.MaxAttempts {
		t.Errorf("expected %d, got %d", fallback.MaxAttempts, merged.MaxAttempts)
	}
	if merged.Backoff != fallback.Backoff {
		t.Errorf("expected %s, got %s", fallback.Backoff, merged.Backoff)
	}
}
