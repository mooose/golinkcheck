package crawler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (c *crawler) processInternal(ctx context.Context, job internalJob) {
	start := time.Now()
	c.emitProgress(job.url)
	parsed, err := url.Parse(job.url)
	if err != nil {
		c.recordError(Error{Source: job.url, Target: job.url, Type: "parse", Message: err.Error()})
		c.savePage(&PageReport{URL: job.url, Error: err.Error()})
		return
	}
	if !c.allowedByRobots(ctx, parsed) {
		c.recordSkippedRobots()
		reason := "blocked by robots.txt"
		page := &PageReport{URL: job.url, Error: reason}
		c.savePage(page)
		c.updateCache(page, time.Now())
		return
	}
	c.recordStatsVisit()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, job.url, nil)
	if err != nil {
		c.recordError(Error{Source: job.url, Target: job.url, Type: "request", Message: err.Error()})
		return
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	if !c.acquireRequestSlot(ctx) {
		reason := "rate limit reached"
		c.recordError(Error{Source: job.url, Target: job.url, Type: "rate", Message: reason})
		page := &PageReport{URL: job.url, Error: reason}
		c.savePage(page)
		c.updateCache(page, time.Now())
		return
	}

	resp, err := c.client.Do(req)
	if err != nil {
		errMsg := err.Error()
		c.recordError(Error{Source: job.url, Target: job.url, Type: "request", Message: errMsg})
		page := &PageReport{URL: job.url, Error: errMsg}
		c.savePage(page)
		c.updateCache(page, time.Now())
		return
	}
	defer resp.Body.Close()

	reader := io.LimitReader(resp.Body, 5*1024*1024)
	body, err := io.ReadAll(reader)
	if err != nil {
		errMsg := err.Error()
		c.recordError(Error{Source: job.url, Target: job.url, Type: "read", Message: errMsg})
		page := &PageReport{URL: job.url, Status: resp.StatusCode, Error: errMsg, Retrieved: time.Since(start)}
		c.savePage(page)
		c.updateCache(page, time.Now())
		return
	}

	links := c.extractLinks(body, job.url)
	if target := extractMetaRefreshTarget(body); target != "" {
		normalized := c.normalizeURL(target)
		if normalized != "" && !linkExists(links, normalized) {
			linkType := LinkTypeExternal
			if parsedTarget, err := url.Parse(normalized); err == nil && strings.EqualFold(parsedTarget.Host, c.start.Host) {
				linkType = LinkTypeInternal
			}
			links = append(links, Link{URL: normalized, Type: linkType})
		}
	}
	pageReport := &PageReport{
		URL:       job.url,
		Status:    resp.StatusCode,
		Links:     links,
		Retrieved: time.Since(start),
	}
	if resp.StatusCode >= 400 {
		msg := fmt.Sprintf("status %d", resp.StatusCode)
		c.recordError(Error{Source: job.url, Target: job.url, Type: "http", Message: msg, Status: resp.StatusCode})
		pageReport.Error = msg
	}

	for _, link := range links {
		switch link.Type {
		case LinkTypeInternal:
			c.recordInternalLink()
			c.enqueueInternal(link.URL, job.depth+1)
		case LinkTypeExternal:
			c.recordExternalLink()
			if c.allowExternal {
				c.enqueueExternal(link.URL, job.url)
			}
		}
	}

	visitedAt := time.Now()
	c.writeMarkdown(pageReport, body, visitedAt)
	c.savePage(pageReport)
	c.updateCache(pageReport, visitedAt)
}

func linkExists(links []Link, target string) bool {
	for _, link := range links {
		if link.URL == target {
			return true
		}
	}
	return false
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
