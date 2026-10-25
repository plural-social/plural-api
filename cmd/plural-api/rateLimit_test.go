package main

import (
	"testing"
	"time"

	"github.com/nicheinc/expect"
)

func Test_ipRateLimiter_allow(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	l := newIPRateLimiter(time.Minute, 2)
	l.now = func() time.Time { return now }

	t.Run("BurstThenDenied", func(t *testing.T) {
		expect.Equal(t, l.allow("1.1.1.1"), true)
		expect.Equal(t, l.allow("1.1.1.1"), true)
		expect.Equal(t, l.allow("1.1.1.1"), false)
	})
	t.Run("IndependentIPs", func(t *testing.T) {
		expect.Equal(t, l.allow("2.2.2.2"), true)
	})
	t.Run("RefillsAfterEvery", func(t *testing.T) {
		now = now.Add(time.Minute)
		expect.Equal(t, l.allow("1.1.1.1"), true)
		expect.Equal(t, l.allow("1.1.1.1"), false)
	})
}
