package crawler

import "net/url"

func (c *crawler) enqueueInternal(raw string) {
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
	if normalized != c.start.String() && c.isCached(normalized) {
		c.recordSkippedCache()
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

	c.internalWG.Add(1)
	if !c.trySendInternal(normalized) {
		go c.waitSendInternal(normalized)
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

func (c *crawler) trySendInternal(pageURL string) bool {
	select {
	case c.internalJobs <- pageURL:
		return true
	default:
		return false
	}
}

func (c *crawler) waitSendInternal(pageURL string) {
	defer func() {
		if recover() != nil {
			c.internalWG.Done()
		}
	}()
	c.internalJobs <- pageURL
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
