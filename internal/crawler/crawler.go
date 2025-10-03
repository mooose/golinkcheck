package crawler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type crawler struct {
	client            *http.Client
	allowExternal     bool
	start             *url.URL
	maxPages          int
	maxDepth          int
	allowedExt        map[string]struct{}
	ignoreRobots      bool
	cachePath         string
	requestsPerMinute int
	progress          func(string)
	markdownDir       string

	internalJobs chan internalJob
	externalJobs chan externalJob

	internalWG sync.WaitGroup
	externalWG sync.WaitGroup

	visitedInternal map[string]struct{}
	visitedExternal map[string]struct{}
	robots          map[string]*robotsGroup

	mu       sync.Mutex
	reportMu sync.Mutex
	pages    map[string]*PageReport
	errors   []Error
	stats    Stats

	cacheMu       sync.RWMutex
	cache         cacheData
	markdownMu    sync.Mutex
	boilerplateMu sync.Mutex
	boilerplates  map[string]*boilerplateInfo

	rateLimiter chan struct{}
	rateTicker  *time.Ticker

	robotsMu sync.Mutex
}

type externalJob struct {
	url    string
	source string
}

// Crawl performs the crawl using the provided configuration and returns a report.
func Crawl(ctx context.Context, cfg Config) (*Report, error) {
	if cfg.StartURL == "" {
		return nil, errors.New("start URL is required")
	}
	parsed, err := url.Parse(cfg.StartURL)
	if err != nil {
		return nil, fmt.Errorf("invalid start URL: %w", err)
	}
	if parsed.Scheme == "" {
		parsed.Scheme = "https"
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("unsupported scheme %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return nil, errors.New("start URL must include a host")
	}

	maxWorkers := cfg.MaxWorkers
	if maxWorkers <= 0 {
		maxWorkers = 8
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}

	if cfg.MaxPages < 0 {
		cfg.MaxPages = 0
	}
	if cfg.RequestsPerMinute < 0 {
		cfg.RequestsPerMinute = 0
	}
	if cfg.RequestsPerMinute == 0 {
		cfg.RequestsPerMinute = 60
	}

	maxDepth := cfg.MaxDepth
	if maxDepth < 0 {
		maxDepth = -1
	}

	allowedExt := buildAllowedExtensions(cfg.AllowedExtensions)

	cachePath := cfg.CachePath

	cacheData, err := loadCache(cachePath)
	if err != nil {
		return nil, fmt.Errorf("load cache: %w", err)
	}

	c := &crawler{
		client:            client,
		allowExternal:     cfg.AllowExternal,
		start:             parsed,
		maxPages:          cfg.MaxPages,
		maxDepth:          maxDepth,
		allowedExt:        allowedExt,
		ignoreRobots:      cfg.IgnoreRobots,
		cachePath:         cachePath,
		requestsPerMinute: cfg.RequestsPerMinute,
		internalJobs:      make(chan internalJob, maxWorkers*2),
		visitedInternal:   map[string]struct{}{},
		visitedExternal:   map[string]struct{}{},
		pages:             map[string]*PageReport{},
		robots:            map[string]*robotsGroup{},
		cache:             cacheData,
		progress:          cfg.Progress,
		markdownDir:       strings.TrimSpace(cfg.MarkdownDir),
		boilerplates:      map[string]*boilerplateInfo{},
	}

	if cachePath != "" {
		c.cacheMu.Lock()
		if c.cache.Visited == nil {
			c.cache.Visited = make(map[string]cacheEntry)
		}
		delete(c.cache.Visited, parsed.String())
		c.cacheMu.Unlock()
	}

	if cfg.AllowExternal {
		c.externalJobs = make(chan externalJob, maxWorkers)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer c.shutdownRateLimiter()
	c.setupRateLimiter(ctx, maxWorkers)

	for i := 0; i < maxWorkers; i++ {
		go c.internalWorker(ctx)
	}

	externalWorkers := maxWorkers / 2
	if externalWorkers < 2 {
		externalWorkers = 2
	}
	if !cfg.AllowExternal {
		externalWorkers = 0
	}
	for i := 0; i < externalWorkers; i++ {
		go c.externalWorker(ctx)
	}

	started := time.Now()
	c.enqueueInternal(parsed.String(), 0)

	c.internalWG.Wait()
	close(c.internalJobs)

	if cfg.AllowExternal {
		c.externalWG.Wait()
		close(c.externalJobs)
	}

	finished := time.Now()

	report := &Report{
		Pages:      c.pages,
		Errors:     c.errors,
		Stats:      c.collectStats(finished.Sub(started)),
		StartedAt:  started,
		FinishedAt: finished,
	}
	if err := c.writeCache(); err != nil {
		return nil, fmt.Errorf("write cache: %w", err)
	}
	return report, nil
}

func (c *crawler) emitProgress(u string) {
	if c.progress == nil {
		return
	}
	c.progress(u)
}
