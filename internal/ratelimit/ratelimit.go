package ratelimit

type Limiter struct{}

// TODO: Implement a proper rate limiter.
func (*Limiter) Limit() bool { return false }
