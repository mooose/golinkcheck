package crawler

import (
	"context"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
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
			MaxDepth:          -1,
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

func TestCrawlRespectsMaxDepth(t *testing.T) {
	t.Parallel()

	client := &http.Client{
		Timeout:   time.Second,
		Transport: depthTransport{maxLevel: 3},
	}

	limited, err := Crawl(context.Background(), Config{
		StartURL:          "https://example.test/start",
		MaxWorkers:        1,
		Client:            client,
		Timeout:           time.Second,
		RequestsPerMinute: 60000,
		IgnoreRobots:      true,
		MaxDepth:          1,
	})
	if err != nil {
		t.Fatalf("crawl with depth limit failed: %v", err)
	}
	if _, ok := limited.Pages["https://example.test/level/2"]; ok {
		t.Fatalf("expected level 2 page to be skipped due to depth limit")
	}
	if limited.Stats.UniqueInternalPages != 2 {
		t.Fatalf("expected two pages with depth limit, got %d", limited.Stats.UniqueInternalPages)
	}
	if limited.Stats.SkippedByDepth == 0 {
		t.Fatalf("expected skipped-by-depth counter to increment")
	}

	unbounded, err := Crawl(context.Background(), Config{
		StartURL:          "https://example.test/start",
		MaxWorkers:        1,
		Client:            client,
		Timeout:           time.Second,
		RequestsPerMinute: 60000,
		IgnoreRobots:      true,
		MaxDepth:          -1,
	})
	if err != nil {
		t.Fatalf("crawl without depth limit failed: %v", err)
	}
	if _, ok := unbounded.Pages["https://example.test/level/3"]; !ok {
		t.Fatalf("expected deepest level to be crawled without limit")
	}
	if unbounded.Stats.SkippedByDepth != 0 {
		t.Fatalf("expected zero depth skips without limit, got %d", unbounded.Stats.SkippedByDepth)
	}
}

func TestCrawlWritesMarkdownFiles(t *testing.T) {
	t.Parallel()

	outputDir := t.TempDir()
	client := &http.Client{
		Timeout:   time.Second,
		Transport: &fakeTransport{linkCount: 2},
	}

	report, err := Crawl(context.Background(), Config{
		StartURL:          "https://example.test/start",
		MaxWorkers:        1,
		Client:            client,
		Timeout:           time.Second,
		RequestsPerMinute: 60000,
		MaxDepth:          -1,
		CachePath:         "",
		IgnoreRobots:      true,
		MarkdownDir:       outputDir,
	})
	if err != nil {
		t.Fatalf("crawl failed: %v", err)
	}
	if report == nil {
		t.Fatal("expected report")
	}

	markdownPath := filepath.Join(outputDir, "example.test", "start.md")
	page := report.Pages["https://example.test/start"]
	if page == nil {
		t.Fatalf("missing page report for start")
	}
	if page.MarkdownPath != markdownPath {
		t.Fatalf("expected markdown path %q, got %q", markdownPath, page.MarkdownPath)
	}
	data, err := os.ReadFile(markdownPath)
	if err != nil {
		t.Fatalf("failed to read markdown output: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "content_sha256:") {
		t.Fatalf("frontmatter missing content hash: %q", content)
	}
	if !strings.Contains(content, "# Welcome") {
		t.Fatalf("expected heading in markdown content: %q", content)
	}
	if !strings.Contains(content, "Site Header") {
		t.Fatalf("expected repeated header in first export: %q", content)
	}
	if !strings.Contains(content, "- [Page 0](https://example.test/page/0)") {
		t.Fatalf("expected list of links in markdown: %q", content)
	}

	page0Path := filepath.Join(outputDir, "example.test", "page", "0.md")
	page0Data, err := os.ReadFile(page0Path)
	if err != nil {
		t.Fatalf("failed to read page markdown: %v", err)
	}
	page0Content := string(page0Data)
	if !strings.Contains(page0Content, "## Page 0") {
		t.Fatalf("expected page-specific heading: %q", page0Content)
	}
	if !strings.Contains(page0Content, "```\ncode block") {
		t.Fatalf("expected code block in markdown export: %q", page0Content)
	}
}

func TestMarkdownExportSkipsUnchangedContent(t *testing.T) {
	outputDir := t.TempDir()
	client := &http.Client{
		Timeout:   time.Second,
		Transport: &fakeTransport{linkCount: 1},
	}

	config := Config{
		StartURL:          "https://example.test/start",
		MaxWorkers:        1,
		Client:            client,
		Timeout:           time.Second,
		RequestsPerMinute: 60000,
		MaxDepth:          -1,
		CachePath:         "",
		IgnoreRobots:      true,
		MarkdownDir:       outputDir,
	}

	if _, err := Crawl(context.Background(), config); err != nil {
		t.Fatalf("initial crawl failed: %v", err)
	}
	markdownPath := filepath.Join(outputDir, "example.test", "start.md")
	sentinel := "\nSENTINEL\n"
	if err := appendToFile(markdownPath, sentinel); err != nil {
		t.Fatalf("failed to append sentinel: %v", err)
	}

	second, err := Crawl(context.Background(), config)
	if err != nil {
		t.Fatalf("second crawl failed: %v", err)
	}
	data, err := os.ReadFile(markdownPath)
	if err != nil {
		t.Fatalf("failed to read markdown after second crawl: %v", err)
	}
	if !strings.Contains(string(data), sentinel) {
		t.Fatalf("expected sentinel to remain in markdown output after unchanged crawl: %q", string(data))
	}
	pageReport := second.Pages["https://example.test/start"]
	if pageReport == nil {
		t.Fatal("missing page report after second crawl")
	}
	if pageReport.MarkdownSkippedReason != "unchanged content" {
		t.Fatalf("expected skipped reason to indicate unchanged content, got %q", pageReport.MarkdownSkippedReason)
	}
}

func TestMarkdownExportRevisitsCachedPagesWithoutExistingFiles(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	cachePath := filepath.Join(tempDir, "cache.json")
	client := &http.Client{
		Timeout:   time.Second,
		Transport: &fakeTransport{linkCount: 2},
	}

	config := Config{
		StartURL:          "https://example.test/start",
		MaxWorkers:        1,
		Client:            client,
		Timeout:           time.Second,
		RequestsPerMinute: 60000,
		MaxDepth:          -1,
		CachePath:         cachePath,
		IgnoreRobots:      true,
	}

	if _, err := Crawl(context.Background(), config); err != nil {
		t.Fatalf("initial crawl failed: %v", err)
	}

	markdownDir := filepath.Join(tempDir, "markdown")
	secondConfig := config
	secondConfig.MarkdownDir = markdownDir

	second, err := Crawl(context.Background(), secondConfig)
	if err != nil {
		t.Fatalf("second crawl failed: %v", err)
	}

	page0Path := filepath.Join(markdownDir, "example.test", "page", "0.md")
	if _, err := os.Stat(page0Path); err != nil {
		t.Fatalf("expected markdown export for cached page, stat failed: %v", err)
	}

	if _, ok := second.Pages["https://example.test/page/0"]; !ok {
		t.Fatalf("expected cached page to be revisited for markdown export")
	}
}

func TestMarkdownExportWritesPlaceholderForEmptyContent(t *testing.T) {
	t.Parallel()

	outputDir := t.TempDir()
	client := &http.Client{
		Timeout:   time.Second,
		Transport: emptyContentTransport{},
	}

	report, err := Crawl(context.Background(), Config{
		StartURL:          "https://example.test/start",
		MaxWorkers:        1,
		Client:            client,
		Timeout:           time.Second,
		RequestsPerMinute: 60000,
		MaxDepth:          -1,
		IgnoreRobots:      true,
		MarkdownDir:       outputDir,
	})
	if err != nil {
		t.Fatalf("crawl failed: %v", err)
	}

	page := report.Pages["https://example.test/start"]
	if page == nil {
		t.Fatal("expected page report")
	}
	if page.MarkdownSkippedReason != "" {
		t.Fatalf("expected markdown export without skip reason, got %q", page.MarkdownSkippedReason)
	}

	path := filepath.Join(outputDir, "example.test", "start.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read markdown output: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "# Empty Page Title") {
		t.Fatalf("expected fallback title in placeholder content, got %q", content)
	}
	if !strings.Contains(content, "This description summarises the page.") {
		t.Fatalf("expected fallback description in placeholder content, got %q", content)
	}
}

func TestEmptyContentFallbackExtractsVisibleText(t *testing.T) {
	input := `<div class="container">
	             <h1>Über das Unternehmen</h1>
	<p>Bereits 1982 wurde hhS von Siegfried Hirsch in Karlsruhe gegründet und hat sich anfangs insbesondere mit 8-Bit Microcontrollern und CPU's beschäftigt. Damit war die Einstieg in Welt der Automatisierung gelegt, der sich in der weiteren Entwicklung von hhS ständig fortgesetzt hat.</p>
	<h3>Weitere Eckpunkte in der Entwicklung von hhS zur heutigen hhS Siegfried Hirsch GmbH &amp; Co. KG</h3>
	<ul>
	<li>Entwicklung und Vertrieb von Microcontrollern zur Steuerung</li>
	<li>Softwareentwicklung für Ersatzteillieferant für Hifi-Elektronik 1983</li>
	<li>Entwicklung einer Microcontroller gesteuerten Einheit für die Materialprüfung</li>
	<li>Softwareentwicklung für Elektronik-Versand in Baden-Baden 1985</li>
	<li>Nutzung für Unix/Solaris für die eigenen Geschäftstätigkeit</li>
	<li>Umzug nach Losheim und Consulting für den KÜS e.V. 1995 im Bereich Internet und Rechenzentrum</li>
	<li>Umzug nach München Büro Brunnthal 2001</li>
	<li>Umzug in die Lipowskystr. 16 in 81373 München 2011</li>
	<li>Umfirmierung zur hhS Siegfried Hirsch GmbH &amp; Co. KG 2017</li>
	<li>40-Jähriges Firmenjubiläum von hhS Siegfried Hirsch im Jahr 2022</li>
	</ul>
	<h2>Aktueller Stand</h2>
	<p>Heute ist die hhS ein innovatives Unternehmen, das im Bereich Digital Industry Automation weltweit tätig ist.</p>
	<p><strong>hhS Siegfried Hirsch GmbH &amp; Co. KG bietet als Systemhaus kundenspezifische Dienstleistungen</strong><br>
	Für die Automatisierung bietet Ihnen hhS international ein umfassendes Portfolio gemäß Ihrer Anforderungen, in vielen Branchen und bietet erstklassige, perfekt aufeinander abgestimmte Produkte, Systeme und Lösungen – abgerundet durch ein komplettes Angebot an Service und Support.</p> 
	        </div>`

	output := buildEmptyContentFallback([]byte(input))
	if !strings.Contains(output, "Bereits 1982 wurde hhS von Siegfried Hirsch") {
		t.Fatalf("expected fallback to include paragraph text, got %q", output)
	}
	if !strings.Contains(output, "- Entwicklung und Vertrieb von Microcontrollern zur Steuerung") {
		t.Fatalf("expected fallback to include list items, got %q", output)
	}
	if strings.Contains(output, "*No textual content extracted.*") {
		t.Fatalf("did not expect placeholder when text is available, got %q", output)
	}
}

func TestMetaRefreshLinkIsFollowed(t *testing.T) {
	t.Parallel()

	outputDir := t.TempDir()
	client := &http.Client{
		Timeout:   time.Second,
		Transport: &fakeTransport{linkCount: 2},
	}

	report, err := Crawl(context.Background(), Config{
		StartURL:          "https://example.test/meta-redirect",
		MaxWorkers:        1,
		Client:            client,
		Timeout:           time.Second,
		RequestsPerMinute: 60000,
		MaxDepth:          -1,
		CachePath:         "",
		IgnoreRobots:      true,
		MarkdownDir:       outputDir,
	})
	if err != nil {
		t.Fatalf("crawl failed: %v", err)
	}

	if _, ok := report.Pages["https://example.test/page/0"]; !ok {
		t.Fatalf("expected meta refresh target to be crawled, report pages: %v", maps.Keys(report.Pages))
	}

	metaPage := report.Pages["https://example.test/meta-redirect"]
	if metaPage == nil {
		t.Fatalf("expected meta refresh source page in report")
	}
	found := false
	for _, link := range metaPage.Links {
		if link.URL == "https://example.test/page/0" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected meta refresh target to be recorded as link, got %+v", metaPage.Links)
	}
}

func appendToFile(path, data string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(data)
	return err
}

type fakeTransport struct {
	linkCount int
}

type emptyContentTransport struct{}
type depthTransport struct {
	maxLevel int
}

func (ft *fakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host != "example.test" {
		return nil, fmt.Errorf("unexpected host: %s", req.URL.Host)
	}

	switch req.URL.Path {
	case "/robots.txt":
		return newStringResponse(req, http.StatusNotFound, ""), nil
	case "/start":
		markup := buildListingPageHTML(ft.linkCount)
		return newStringResponse(req, http.StatusOK, markup), nil
	case "/meta-redirect":
		markup := "<!doctype html><html><head><title>Redirecting</title><meta http-equiv=\"refresh\" content=\"0; url=/page/0\"></head><body><p>Redirecting...</p></body></html>"
		return newStringResponse(req, http.StatusOK, markup), nil
	default:
		if strings.HasPrefix(req.URL.Path, "/page/") {
			pageID := strings.TrimPrefix(req.URL.Path, "/page/")
			markup := buildDetailPageHTML(pageID)
			return newStringResponse(req, http.StatusOK, markup), nil
		}
		return newStringResponse(req, http.StatusNotFound, ""), nil
	}
}

func (emptyContentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host != "example.test" {
		return nil, fmt.Errorf("unexpected host: %s", req.URL.Host)
	}

	switch req.URL.Path {
	case "/robots.txt":
		return newStringResponse(req, http.StatusNotFound, ""), nil
	case "/start":
		markup := "<!doctype html><html><head><title>Empty Page Title</title><meta name=\"description\" content=\"This description summarises the page.\"></head><body></body></html>"
		return newStringResponse(req, http.StatusOK, markup), nil
	default:
		return newStringResponse(req, http.StatusNotFound, ""), nil
	}
}

func (dt depthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host != "example.test" {
		return nil, fmt.Errorf("unexpected host: %s", req.URL.Host)
	}

	switch req.URL.Path {
	case "/robots.txt":
		return newStringResponse(req, http.StatusNotFound, ""), nil
	case "/start":
		return newStringResponse(req, http.StatusOK, buildDepthPageHTML(0, dt.maxLevel)), nil
	default:
		if strings.HasPrefix(req.URL.Path, "/level/") {
			value := strings.TrimPrefix(req.URL.Path, "/level/")
			level, err := strconv.Atoi(value)
			if err != nil {
				return newStringResponse(req, http.StatusNotFound, ""), nil
			}
			if level < 0 {
				return newStringResponse(req, http.StatusNotFound, ""), nil
			}
			return newStringResponse(req, http.StatusOK, buildDepthPageHTML(level, dt.maxLevel)), nil
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

func buildListingPageHTML(linkCount int) string {
	var sb strings.Builder
	sb.WriteString("<html><body>")
	sb.WriteString(siteHeader())
	sb.WriteString("<main>")
	sb.WriteString("<h1>Welcome</h1>")
	sb.WriteString("<p>Welcome to the example site.</p>")
	sb.WriteString("<ul>")
	for i := 0; i < linkCount; i++ {
		fmt.Fprintf(&sb, `<li><a href="/page/%d">Page %d</a></li>`, i, i)
	}
	sb.WriteString("</ul>")
	sb.WriteString("</main>")
	sb.WriteString(siteFooter())
	sb.WriteString("</body></html>")
	return sb.String()
}

func buildDetailPageHTML(id string) string {
	var sb strings.Builder
	sb.WriteString("<html><body>")
	sb.WriteString(siteHeader())
	sb.WriteString("<main>")
	fmt.Fprintf(&sb, "<h2>Page %s</h2>", id)
	fmt.Fprintf(&sb, "<p>Details for page %s.</p>", id)
	sb.WriteString("<pre><code>code block for page \n")
	sb.WriteString(id)
	sb.WriteString("</code></pre>")
	sb.WriteString("</main>")
	sb.WriteString(siteFooter())
	sb.WriteString("</body></html>")
	return sb.String()
}

func buildDepthPageHTML(level, maxLevel int) string {
	var sb strings.Builder
	sb.WriteString("<!doctype html><html><body>")
	fmt.Fprintf(&sb, "<h1>Level %d</h1>", level)
	if level < maxLevel {
		next := level + 1
		fmt.Fprintf(&sb, `<a href="/level/%d">Go to level %d</a>`, next, next)
	}
	sb.WriteString("</body></html>")
	return sb.String()
}

func siteHeader() string {
	return `<header><div class="branding">Site Header</div><nav><ul><li><a href="/start">Home</a></li></ul></nav></header>`
}

func siteFooter() string {
	return `<footer><p>Site Footer</p></footer>`
}
