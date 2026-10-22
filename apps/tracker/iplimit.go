package main

import (
	"hash/maphash"
	"net"
	"sync"
	"time"
)

// clickLimiter caps clicks per client IP in this process (audit SEC-4):
// a fixed window of a minute, no Redis — a network round trip per click
// would break the redirect budget (CLAUDE.md #9). Each replica counts on
// its own, so N replicas allow up to N× the limit; it is a floor against
// one source flooding the tracker, not a precise quota.
//
// Windows are aligned to the clock: when one ends, a shard drops every
// count at once, so memory holds only the IPs seen this minute. A source
// can get up to twice the limit across a window boundary.
//
// IPv6 is counted per /64: one subscriber usually holds the whole prefix.
type clickLimiter struct {
	limit  int // per window; 0 or less: no limit
	window time.Duration
	seed   maphash.Seed
	shards [64]limitShard
}

type limitShard struct {
	mu     sync.Mutex
	epoch  int64
	counts map[string]int
}

func newClickLimiter(limit int, window time.Duration) *clickLimiter {
	l := &clickLimiter{limit: limit, window: window, seed: maphash.MakeSeed()}
	for i := range l.shards {
		l.shards[i].counts = map[string]int{}
	}
	return l
}

// allow counts one click from ip and reports whether it is within the
// limit. A nil limiter, no limit or no IP allows everything.
func (l *clickLimiter) allow(ip net.IP, now time.Time) bool {
	if l == nil || l.limit <= 0 || ip == nil {
		return true
	}
	key := limitKey(ip)
	s := &l.shards[maphash.String(l.seed, key)%uint64(len(l.shards))]
	epoch := now.UnixNano() / int64(l.window)

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.epoch != epoch {
		clear(s.counts)
		s.epoch = epoch
	}
	n := s.counts[key] + 1
	s.counts[key] = n
	return n <= l.limit
}

// limitKey is the IPv4 address, or the IPv6 /64 the address is in.
func limitKey(ip net.IP) string {
	if v4 := ip.To4(); v4 != nil {
		return string(v4)
	}
	return string(ip.To16()[:8])
}
