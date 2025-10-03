package crawler

import (
	"net/url"
	"os"
)

type internalJob struct {
	url   string
	depth int
}

func (c *crawler) enqueueInternal(raw string, depth int) {
	normalized := c.normalizeURL(raw)
	if normalized == "" {
		return
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return
	}
	if !c.allowedExtension(parsed) {
		c.recordSkippedExtension()
		return
	}
	if normalized != c.start.String() && c.shouldSkipCached(normalized) {
		c.recordSkippedCache()
		return
	}
	if c.maxDepth >= 0 && depth > c.maxDepth {
		c.recordSkippedDepth()
		return
	}
	c.mu.Lock()
	if c.maxPages > 0 && len(c.visitedInternal) >= c.maxPages {
		c.mu.Unlock()
		c.recordSkippedLimit()
		return
	}
	if _, seen := c.visitedInternal[normalized]; seen {
		c.mu.Unlock()
		return
	}
	c.visitedInternal[normalized] = struct{}{}
	c.mu.Unlock()

	job := internalJob{url: normalized, depth: depth}
	c.internalWG.Add(1)
	if !c.trySendInternal(job) {
		go c.waitSendInternal(job)
	}
}

func (c *crawler) enqueueExternal(raw, source string) {
	normalized := c.normalizeURL(raw)
	if normalized == "" {
		return
	}
	c.mu.Lock()
	if _, seen := c.visitedExternal[normalized]; seen {
		c.mu.Unlock()
		return
	}
	c.visitedExternal[normalized] = struct{}{}
	c.mu.Unlock()

	c.externalWG.Add(1)
	job := externalJob{url: normalized, source: source}
	if !c.trySendExternal(job) {
		go c.waitSendExternal(job)
	}
}

func (c *crawler) trySendInternal(job internalJob) bool {
	select {
	case c.internalJobs <- job:
		return true
	default:
		return false
	}
}

func (c *crawler) waitSendInternal(job internalJob) {
	defer func() {
		if recover() != nil {
			c.internalWG.Done()
		}
	}()
	c.internalJobs <- job
}

func (c *crawler) trySendExternal(job externalJob) bool {
	select {
	case c.externalJobs <- job:
		return true
	default:
		return false
	}
}

func (c *crawler) waitSendExternal(job externalJob) {
	defer func() {
		if recover() != nil {
			c.externalWG.Done()
		}
	}()
	c.externalJobs <- job
}

func (c *crawler) shouldSkipCached(normalized string) bool {
	if !c.isCached(normalized) {
		return false
	}
	if c.markdownDir == "" {
		return true
	}
	target, err := c.markdownFilePath(normalized)
	if err != nil {
		return true
	}
	if _, err := os.Stat(target); err != nil {
		return false
	}
	return true
}
