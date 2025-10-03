package crawler

import "time"

func (c *crawler) recordStatsVisit() {
	c.mu.Lock()
	c.stats.PagesVisited++
	c.mu.Unlock()
}

func (c *crawler) recordExternalChecked() {
	c.mu.Lock()
	c.stats.ExternalLinksChecked++
	c.mu.Unlock()
}

func (c *crawler) recordInternalLink() {
	c.mu.Lock()
	c.stats.TotalInternalLinks++
	c.mu.Unlock()
}

func (c *crawler) recordExternalLink() {
	c.mu.Lock()
	c.stats.TotalExternalLinks++
	c.mu.Unlock()
}

func (c *crawler) recordSkippedCache() {
	c.mu.Lock()
	c.stats.SkippedByCache++
	c.mu.Unlock()
}

func (c *crawler) recordSkippedRobots() {
	c.mu.Lock()
	c.stats.SkippedByRobots++
	c.mu.Unlock()
}

func (c *crawler) recordSkippedExtension() {
	c.mu.Lock()
	c.stats.SkippedByExtension++
	c.mu.Unlock()
}

func (c *crawler) recordSkippedLimit() {
	c.mu.Lock()
	c.stats.SkippedByLimit++
	c.mu.Unlock()
}

func (c *crawler) recordSkippedDepth() {
	c.mu.Lock()
	c.stats.SkippedByDepth++
	c.mu.Unlock()
}

func (c *crawler) collectStats(duration time.Duration) Stats {
	c.mu.Lock()
	defer c.mu.Unlock()
	stats := c.stats
	stats.UniqueInternalPages = len(c.visitedInternal)
	stats.UniqueExternalLinks = len(c.visitedExternal)
	stats.Duration = duration
	return stats
}
