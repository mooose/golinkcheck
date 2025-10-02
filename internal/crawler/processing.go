package crawler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

func (c *crawler) processInternal(ctx context.Context, pageURL string) {
	start := time.Now()
	c.emitProgress(pageURL)
	parsed, err := url.Parse(pageURL)
	if err != nil {
		c.recordError(Error{Source: pageURL, Target: pageURL, Type: "parse", Message: err.Error()})
		c.savePage(&PageReport{URL: pageURL, Error: err.Error()})
		return
	}
	if !c.allowedByRobots(ctx, parsed) {
		c.recordSkippedRobots()
		reason := "blocked by robots.txt"
		page := &PageReport{URL: pageURL, Error: reason}
		c.savePage(page)
		c.updateCache(page, time.Now())
		return
	}
	c.recordStatsVisit()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		c.recordError(Error{Source: pageURL, Target: pageURL, Type: "request", Message: err.Error()})
		return
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	if !c.acquireRequestSlot(ctx) {
		reason := "rate limit reached"
		c.recordError(Error{Source: pageURL, Target: pageURL, Type: "rate", Message: reason})
		page := &PageReport{URL: pageURL, Error: reason}
		c.savePage(page)
		c.updateCache(page, time.Now())
		return
	}

	resp, err := c.client.Do(req)
	if err != nil {
		errMsg := err.Error()
		c.recordError(Error{Source: pageURL, Target: pageURL, Type: "request", Message: errMsg})
		page := &PageReport{URL: pageURL, Error: errMsg}
		c.savePage(page)
		c.updateCache(page, time.Now())
		return
	}
	defer resp.Body.Close()

	reader := io.LimitReader(resp.Body, 5*1024*1024)
	body, err := io.ReadAll(reader)
	if err != nil {
		errMsg := err.Error()
		c.recordError(Error{Source: pageURL, Target: pageURL, Type: "read", Message: errMsg})
		page := &PageReport{URL: pageURL, Status: resp.StatusCode, Error: errMsg, Retrieved: time.Since(start)}
		c.savePage(page)
		c.updateCache(page, time.Now())
		return
	}

	links := c.extractLinks(body, pageURL)
	pageReport := &PageReport{
		URL:       pageURL,
		Status:    resp.StatusCode,
		Links:     links,
		Retrieved: time.Since(start),
	}
	if resp.StatusCode >= 400 {
		msg := fmt.Sprintf("status %d", resp.StatusCode)
		c.recordError(Error{Source: pageURL, Target: pageURL, Type: "http", Message: msg, Status: resp.StatusCode})
		pageReport.Error = msg
	}

	for _, link := range links {
		switch link.Type {
		case LinkTypeInternal:
			c.recordInternalLink()
			c.enqueueInternal(link.URL)
		case LinkTypeExternal:
			c.recordExternalLink()
			if c.allowExternal {
				c.enqueueExternal(link.URL, pageURL)
			}
		}
	}

	visitedAt := time.Now()
	c.savePage(pageReport)
	c.updateCache(pageReport, visitedAt)
}

func (c *crawler) processExternal(ctx context.Context, job externalJob) {
	c.emitProgress(job.url)
	parsed, err := url.Parse(job.url)
	if err != nil {
		c.recordError(Error{Source: job.source, Target: job.url, Type: "parse", Message: err.Error()})
		return
	}
	if !c.allowedByRobots(ctx, parsed) {
		c.recordSkippedRobots()
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, job.url, nil)
	if err != nil {
		c.recordError(Error{Source: job.source, Target: job.url, Type: "request", Message: err.Error()})
		return
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	if !c.acquireRequestSlot(ctx) {
		c.recordError(Error{Source: job.source, Target: job.url, Type: "rate", Message: "rate limit reached"})
		return
	}

	resp, err := c.client.Do(req)
	if err != nil {
		c.recordError(Error{Source: job.source, Target: job.url, Type: "request", Message: err.Error()})
		return
	}
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		c.recordError(Error{Source: job.source, Target: job.url, Type: "http", Message: fmt.Sprintf("status %d", resp.StatusCode), Status: resp.StatusCode})
	}

	c.recordExternalChecked()
}
