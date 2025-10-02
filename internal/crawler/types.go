package crawler

import (
	"net/http"
	"time"
)

const defaultUserAgent = "linkcheck-bot/1.0"

// Config defines inputs for the crawler.
type Config struct {
	StartURL          string
	AllowExternal     bool
	MaxWorkers        int
	Client            *http.Client
	Timeout           time.Duration
	MaxPages          int
	RequestsPerMinute int
	AllowedExtensions []string
	IgnoreRobots      bool
	CachePath         string
	Progress          func(string)
}

// Report captures the outcome of a crawl.
type Report struct {
	Pages      map[string]*PageReport
	Errors     []Error
	Stats      Stats
	StartedAt  time.Time
	FinishedAt time.Time
}

// PageReport summarizes the crawl result for one page.
type PageReport struct {
	URL       string
	Status    int
	Error     string
	Links     []Link
	Retrieved time.Duration
}

// Link describes a discovered link and its classification.
type Link struct {
	URL  string
	Type LinkType
}

// LinkType describes the classification of a link.
type LinkType string

const (
	// LinkTypeInternal indicates a link that belongs to the starting host.
	LinkTypeInternal LinkType = "internal"
	// LinkTypeExternal indicates a link that targets another host.
	LinkTypeExternal LinkType = "external"
)

// Error captures a failure that occurred when visiting or validating a link.
type Error struct {
	Source  string
	Target  string
	Type    string
	Message string
	Status  int
}

// Stats aggregates crawl level counters.
type Stats struct {
	PagesVisited         int
	UniqueInternalPages  int
	UniqueExternalLinks  int
	TotalInternalLinks   int
	TotalExternalLinks   int
	ExternalLinksChecked int
	Duration             time.Duration
	SkippedByCache       int
	SkippedByRobots      int
	SkippedByExtension   int
	SkippedByLimit       int
}
