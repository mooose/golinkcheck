package crawler

import (
	"context"
	"time"
)

func (c *crawler) setupRateLimiter(ctx context.Context, maxWorkers int) {
	rpm := c.requestsPerMinute
	if rpm <= 0 {
		return
	}
	interval := time.Minute / time.Duration(rpm)
	if interval <= 0 {
		interval = time.Minute
	}
	capacity := rpm
	if capacity < maxWorkers {
		capacity = maxWorkers
	}
	c.rateLimiter = make(chan struct{}, capacity)
	fill := maxWorkers
	if fill > capacity {
		fill = capacity
	}
	for i := 0; i < fill; i++ {
		c.rateLimiter <- struct{}{}
	}
	c.rateTicker = time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-c.rateTicker.C:
				select {
				case c.rateLimiter <- struct{}{}:
				default:
				}
			}
		}
	}()
}

func (c *crawler) shutdownRateLimiter() {
	if c.rateTicker != nil {
		c.rateTicker.Stop()
		c.rateTicker = nil
	}
}

func (c *crawler) acquireRequestSlot(ctx context.Context) bool {
	if c.rateLimiter == nil {
		return true
	}
	select {
	case <-ctx.Done():
		return false
	case <-c.rateLimiter:
		return true
	}
}
