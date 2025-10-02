package crawler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type crawlResult struct {
	report *Report
	err    error
}

func TestCrawlHandlesManyQueuedInternalLinks(t *testing.T) {
	t.Parallel()

	const linkCount = 50

	client := &http.Client{
		Timeout:   time.Second,
		Transport: &fakeTransport{linkCount: linkCount},
	}

	ctx := context.Background()
	resultCh := make(chan crawlResult, 1)

	go func() {
		report, err := Crawl(ctx, Config{
			StartURL:          "https://example.test/start",
			MaxWorkers:        1,
			Client:            client,
			Timeout:           time.Second,
			RequestsPerMinute: 60000,
			CachePath:         "",
			IgnoreRobots:      true,
		})
		resultCh <- crawlResult{report: report, err: err}
	}()

	select {
	case <-time.After(2 * time.Second):
		t.Fatal("crawl did not finish")
	case res := <-resultCh:
		if res.err != nil {
			t.Fatalf("crawl failed: %v", res.err)
		}
		if res.report == nil {
			t.Fatal("report is nil")
		}
		if got, want := res.report.Stats.UniqueInternalPages, linkCount+1; got != want {
			t.Fatalf("unexpected page count: got %d want %d", got, want)
		}
	}
}

type fakeTransport struct {
	linkCount int
}

func (ft *fakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host != "example.test" {
		return nil, fmt.Errorf("unexpected host: %s", req.URL.Host)
	}

	switch req.URL.Path {
	case "/robots.txt":
		return newStringResponse(req, http.StatusNotFound, ""), nil
	case "/start":
		var sb strings.Builder
		sb.WriteString("<html><body>")
		for i := 0; i < ft.linkCount; i++ {
			fmt.Fprintf(&sb, `<a href="/page/%d">page %d</a>`, i, i)
		}
		sb.WriteString("</body></html>")
		return newStringResponse(req, http.StatusOK, sb.String()), nil
	default:
		if strings.HasPrefix(req.URL.Path, "/page/") {
			body := fmt.Sprintf("<html><body>%s</body></html>", req.URL.Path)
			return newStringResponse(req, http.StatusOK, body), nil
		}
		return newStringResponse(req, http.StatusNotFound, ""), nil
	}
}

func newStringResponse(req *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
		Request:    req,
	}
}
