package crawler

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
)

const (
	maxHeaderLines          = 6
	maxFooterLines          = 6
	boilerplateConfirmCount = 3
)

type boilerplateInfo struct {
	headerCandidate []string
	footerCandidate []string
	headerConfirmed bool
	footerConfirmed bool
	headerMatches   int
	footerMatches   int
}

func (c *crawler) writeMarkdown(page *PageReport, body []byte, visitedAt time.Time) {
	if c == nil || c.markdownDir == "" || page == nil {
		return
	}

	parsedURL, err := url.Parse(page.URL)
	if err != nil {
		return
	}

	converter := htmltomarkdown.NewConverter(parsedURL.String(), true, nil)
	markdown, err := converter.ConvertString(string(body))
	if err != nil {
		c.recordError(Error{Source: page.URL, Target: page.URL, Type: "markdown", Message: err.Error()})
		return
	}

	trimmed := strings.TrimSpace(markdown)
	text := trimmed
	if trimmed != "" {
		cleaned := strings.TrimSpace(c.removeBoilerplate(parsedURL, trimmed))
		if cleaned != "" {
			text = cleaned
		}
	} else {
		text = buildEmptyContentFallback(body)
	}

	sum := sha256.Sum256([]byte(text))
	hash := hex.EncodeToString(sum[:])

	target, err := c.markdownFilePath(page.URL)
	if err != nil {
		c.recordError(Error{Source: page.URL, Target: page.URL, Type: "markdown", Message: err.Error()})
		page.MarkdownSkippedReason = err.Error()
		return
	}

	c.markdownMu.Lock()
	defer c.markdownMu.Unlock()

	if existingHash, err := readMarkdownHash(target); err == nil && existingHash == hash {
		page.MarkdownSkippedReason = "unchanged content"
		return
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		c.recordError(Error{Source: page.URL, Target: target, Type: "markdown", Message: err.Error()})
		page.MarkdownSkippedReason = err.Error()
		return
	}

	internalLinks, externalLinks := countLinkTypes(page.Links)
	content := buildMarkdownDocument(page, text, visitedAt, hash, internalLinks, externalLinks)
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		c.recordError(Error{Source: page.URL, Target: target, Type: "markdown", Message: err.Error()})
		page.MarkdownSkippedReason = err.Error()
	}
	page.MarkdownPath = target
	page.MarkdownSkippedReason = ""
}

func countLinkTypes(links []Link) (int, int) {
	var internal, external int
	for _, link := range links {
		switch link.Type {
		case LinkTypeInternal:
			internal++
		case LinkTypeExternal:
			external++
		}
	}
	return internal, external
}

func buildMarkdownDocument(page *PageReport, body string, visitedAt time.Time, hash string, internalLinks, externalLinks int) string {
	builder := strings.Builder{}
	builder.Grow(len(body) + 256)
	builder.WriteString("---\n")
	builder.WriteString(fmt.Sprintf("url: %s\n", page.URL))
	builder.WriteString(fmt.Sprintf("status: %d\n", page.Status))
	builder.WriteString(fmt.Sprintf("retrieved_ms: %d\n", page.Retrieved.Milliseconds()))
	builder.WriteString(fmt.Sprintf("fetched_at: %s\n", visitedAt.UTC().Format(time.RFC3339)))
	builder.WriteString(fmt.Sprintf("content_sha256: %s\n", hash))
	builder.WriteString(fmt.Sprintf("word_count: %d\n", wordCount(body)))
	builder.WriteString(fmt.Sprintf("internal_links: %d\n", internalLinks))
	builder.WriteString(fmt.Sprintf("external_links: %d\n", externalLinks))
	if page.Error != "" {
		builder.WriteString(fmt.Sprintf("error: %q\n", page.Error))
	}
	builder.WriteString("---\n\n")
	builder.WriteString(body)
	builder.WriteString("\n")
	return builder.String()
}

func wordCount(body string) int {
	if body == "" {
		return 0
	}
	return len(strings.Fields(body))
}

func (c *crawler) markdownFilePath(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	host := sanitizeSegment(parsed.Host)
	if host == "" {
		host = "unknown-host"
	}

	segments := make([]string, 0, 4)
	trimmedPath := strings.Trim(parsed.Path, "/")
	if trimmedPath == "" {
		segments = append(segments, "index")
	} else {
		for _, part := range strings.Split(trimmedPath, "/") {
			part = sanitizeSegment(part)
			if part == "" {
				part = "section"
			}
			segments = append(segments, part)
		}
	}

	if len(segments) == 0 {
		segments = append(segments, "index")
	}

	base := segments[len(segments)-1]
	if parsed.RawQuery != "" {
		base = fmt.Sprintf("%s__%s", base, sanitizeSegment(parsed.RawQuery))
	}
	segments[len(segments)-1] = base + ".md"

	parts := append([]string{c.markdownDir, host}, segments...)
	return filepath.Join(parts...), nil
}

func sanitizeSegment(input string) string {
	if input == "" {
		return ""
	}
	var builder strings.Builder
	builder.Grow(len(input))
	for _, r := range input {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			builder.WriteRune(r)
		default:
			builder.WriteRune('-')
		}
	}
	return strings.Trim(builder.String(), "-")
}

func readMarkdownHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(content, "---\n") {
		return "", fmt.Errorf("missing frontmatter")
	}
	remainder := content[4:]
	end := strings.Index(remainder, "\n---")
	if end == -1 {
		return "", fmt.Errorf("missing closing frontmatter")
	}
	frontmatter := remainder[:end]
	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "content_sha256:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1]), nil
			}
		}
	}
	return "", fmt.Errorf("content hash not found")
}

var (
	titlePattern           = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	metaDescriptionPattern = regexp.MustCompile(`(?is)<meta[^>]+(?:name|property)\s*=\s*['"](?:description|og:description)['"][^>]*content\s*=\s*['"]([^'"]+)['"]`)
	metaRefreshPattern     = regexp.MustCompile(`(?is)<meta[^>]+http-equiv\s*=\s*['"]refresh['"][^>]*content\s*=\s*['"]([^'"]+)['"]`)
)

var (
	fallbackStripPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`),
		regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`),
		regexp.MustCompile(`(?is)<noscript[^>]*>.*?</noscript>`),
		regexp.MustCompile(`(?is)<template[^>]*>.*?</template>`),
		regexp.MustCompile(`(?is)<iframe[^>]*>.*?</iframe>`),
	}
	fallbackCommentPattern  = regexp.MustCompile(`(?s)<!--.*?-->`)
	fallbackBROpenPattern   = regexp.MustCompile(`(?is)<br[^>]*>`)
	fallbackLiOpenPattern   = regexp.MustCompile(`(?is)<li[^>]*>`)
	fallbackLiClosePattern  = regexp.MustCompile(`(?is)</li>`)
	fallbackHeadingPatterns = []struct {
		re     *regexp.Regexp
		prefix string
	}{
		{regexp.MustCompile(`(?is)<h1[^>]*>`), "\n\n# "},
		{regexp.MustCompile(`(?is)<h2[^>]*>`), "\n\n## "},
		{regexp.MustCompile(`(?is)<h3[^>]*>`), "\n\n### "},
		{regexp.MustCompile(`(?is)<h4[^>]*>`), "\n\n#### "},
		{regexp.MustCompile(`(?is)<h5[^>]*>`), "\n\n##### "},
		{regexp.MustCompile(`(?is)<h6[^>]*>`), "\n\n###### "},
	}
	fallbackHeadingClosePattern = regexp.MustCompile(`(?is)</h[1-6]>`)
	fallbackBlockClosePattern   = regexp.MustCompile(`(?is)</(p|div|section|article|main|header|footer|address|blockquote|table|tr|tbody|thead|tfoot|ul|ol)>`)
	fallbackBlockOpenPattern    = regexp.MustCompile(`(?is)<(p|div|section|article|main|header|footer|address|blockquote|table|tr|tbody|thead|tfoot|ul|ol)[^>]*>`)
	fallbackTagPattern          = regexp.MustCompile(`(?is)<[^>]+>`)
)

func buildEmptyContentFallback(body []byte) string {
	src := string(body)
	title := extractHTMLTitle(src)
	description := extractMetaDescription(src)
	redirect := extractMetaRefreshTargetFromString(src)

	var builder strings.Builder
	if title != "" {
		builder.WriteString("# ")
		builder.WriteString(title)
		builder.WriteString("\n\n")
	}
	if description != "" {
		builder.WriteString(description)
		builder.WriteString("\n\n")
	}
	if redirect != "" {
		builder.WriteString("Meta refresh redirect target: ")
		builder.WriteString(redirect)
		builder.WriteString("\n\n")
	}
	visible := extractVisibleText(src)
	if visible != "" {
		builder.WriteString(visible)
		if !strings.HasSuffix(visible, "\n") {
			builder.WriteString("\n")
		}
		return builder.String()
	}
	builder.WriteString("*No textual content extracted.*\n")
	return builder.String()
}

func extractHTMLTitle(src string) string {
	m := titlePattern.FindStringSubmatch(src)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(html.UnescapeString(m[1]))
}

func extractMetaDescription(src string) string {
	m := metaDescriptionPattern.FindStringSubmatch(src)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(html.UnescapeString(m[1]))
}

func extractMetaRefreshTarget(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	return extractMetaRefreshTargetFromString(string(body))
}

func extractMetaRefreshTargetFromString(src string) string {
	m := metaRefreshPattern.FindStringSubmatch(src)
	if len(m) < 2 {
		return ""
	}
	content := strings.TrimSpace(m[1])
	parts := strings.Split(content, ";")
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(trimmed), "url=") {
			target := strings.TrimSpace(trimmed[4:])
			target = strings.Trim(target, "'\"")
			return html.UnescapeString(target)
		}
	}
	return ""
}

func extractVisibleText(src string) string {
	if strings.TrimSpace(src) == "" {
		return ""
	}
	cleaned := src
	for _, pattern := range fallbackStripPatterns {
		cleaned = pattern.ReplaceAllString(cleaned, " ")
	}
	cleaned = fallbackCommentPattern.ReplaceAllString(cleaned, " ")
	cleaned = fallbackBROpenPattern.ReplaceAllString(cleaned, "\n")
	cleaned = fallbackLiOpenPattern.ReplaceAllString(cleaned, "\n- ")
	cleaned = fallbackLiClosePattern.ReplaceAllString(cleaned, "")
	for _, entry := range fallbackHeadingPatterns {
		cleaned = entry.re.ReplaceAllString(cleaned, entry.prefix)
	}
	cleaned = fallbackHeadingClosePattern.ReplaceAllString(cleaned, "\n\n")
	cleaned = fallbackBlockOpenPattern.ReplaceAllString(cleaned, "\n\n")
	cleaned = fallbackBlockClosePattern.ReplaceAllString(cleaned, "\n\n")
	cleaned = fallbackTagPattern.ReplaceAllString(cleaned, "")
	cleaned = html.UnescapeString(cleaned)

	lines := strings.Split(cleaned, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if len(result) == 0 || result[len(result)-1] == "" {
				continue
			}
			result = append(result, "")
			continue
		}
		normalized := normalizeSpaces(trimmed)
		if normalized == "" {
			continue
		}
		result = append(result, normalized)
	}
	output := strings.TrimSpace(strings.Join(result, "\n"))
	if output == "" {
		return ""
	}
	return output + "\n"
}

func normalizeSpaces(line string) string {
	if strings.HasPrefix(line, "- ") {
		collapsed := collapseUnicodeSpaces(line[2:])
		if collapsed == "" {
			return ""
		}
		return "- " + collapsed
	}
	if strings.HasPrefix(line, "#") {
		sharpCount := 0
		for _, r := range line {
			if r == '#' {
				sharpCount++
			} else {
				break
			}
		}
		if sharpCount == 0 {
			return collapseUnicodeSpaces(line)
		}
		remainder := strings.TrimSpace(line[sharpCount:])
		if remainder == "" {
			return strings.Repeat("#", sharpCount)
		}
		return strings.Repeat("#", sharpCount) + " " + collapseUnicodeSpaces(remainder)
	}
	return collapseUnicodeSpaces(line)
}

func collapseUnicodeSpaces(input string) string {
	var builder strings.Builder
	lastWasSpace := false
	for _, r := range input {
		if unicode.IsSpace(r) {
			if lastWasSpace {
				continue
			}
			builder.WriteRune(' ')
			lastWasSpace = true
			continue
		}
		builder.WriteRune(r)
		lastWasSpace = false
	}
	return strings.TrimSpace(builder.String())
}

func (c *crawler) removeBoilerplate(parsed *url.URL, markdown string) string {
	if parsed == nil {
		return markdown
	}
	lines := splitLines(markdown)
	if len(lines) == 0 {
		return markdown
	}
	host := strings.ToLower(parsed.Host)
	c.boilerplateMu.Lock()
	if c.boilerplates == nil {
		c.boilerplates = make(map[string]*boilerplateInfo)
	}
	info, ok := c.boilerplates[host]
	if !ok {
		header := collectHeaderCandidate(lines)
		footer := collectFooterCandidate(lines)
		c.boilerplates[host] = &boilerplateInfo{
			headerCandidate: header,
			footerCandidate: footer,
		}
		c.boilerplateMu.Unlock()
		return markdown
	}

	updatedLines := lines
	if len(info.headerCandidate) > 0 {
		if info.headerConfirmed {
			updatedLines, _ = matchAndRemoveHeader(updatedLines, info.headerCandidate)
		} else if matchesHeader(lines, info.headerCandidate) {
			info.headerMatches++
			if info.headerMatches >= boilerplateConfirmCount {
				info.headerConfirmed = true
				updatedLines, _ = matchAndRemoveHeader(updatedLines, info.headerCandidate)
			}
		} else {
			info.headerCandidate = collectHeaderCandidate(lines)
			info.headerMatches = 0
		}
	}
	if len(info.footerCandidate) > 0 {
		if info.footerConfirmed {
			updatedLines, _ = matchAndRemoveFooter(updatedLines, info.footerCandidate)
		} else if matchesFooter(lines, info.footerCandidate) {
			info.footerMatches++
			if info.footerMatches >= boilerplateConfirmCount {
				info.footerConfirmed = true
				updatedLines, _ = matchAndRemoveFooter(updatedLines, info.footerCandidate)
			}
		} else {
			info.footerCandidate = collectFooterCandidate(lines)
			info.footerMatches = 0
		}
	}
	c.boilerplateMu.Unlock()
	trimmed := trimEmptyEdges(updatedLines)
	return strings.Join(trimmed, "\n")
}

func splitLines(input string) []string {
	if input == "" {
		return nil
	}
	return strings.Split(input, "\n")
}

func collectHeaderCandidate(lines []string) []string {
	var candidate []string
	count := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if len(candidate) == 0 {
				continue
			}
			break
		}
		candidate = append(candidate, trimmed)
		count++
		if count >= maxHeaderLines {
			break
		}
	}
	return candidate
}

func collectFooterCandidate(lines []string) []string {
	var candidate []string
	count := 0
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			if len(candidate) == 0 {
				continue
			}
			break
		}
		candidate = append(candidate, trimmed)
		count++
		if count >= maxFooterLines {
			break
		}
	}
	if len(candidate) == 0 {
		return candidate
	}
	for i, j := 0, len(candidate)-1; i < j; i, j = i+1, j-1 {
		candidate[i], candidate[j] = candidate[j], candidate[i]
	}
	return candidate
}

func matchesHeader(lines, header []string) bool {
	if len(header) == 0 {
		return false
	}
	idx := 0
	for idx < len(lines) && strings.TrimSpace(lines[idx]) == "" {
		idx++
	}
	for _, expected := range header {
		if idx >= len(lines) {
			return false
		}
		if strings.TrimSpace(lines[idx]) != expected {
			return false
		}
		idx++
	}
	return true
}

func matchesFooter(lines, footer []string) bool {
	if len(footer) == 0 {
		return false
	}
	idx := len(lines) - 1
	for idx >= 0 && strings.TrimSpace(lines[idx]) == "" {
		idx--
	}
	for i := len(footer) - 1; i >= 0; i-- {
		if idx < 0 {
			return false
		}
		if strings.TrimSpace(lines[idx]) != footer[i] {
			return false
		}
		idx--
	}
	return true
}

func matchAndRemoveHeader(lines []string, header []string) ([]string, bool) {
	if len(header) == 0 {
		return lines, false
	}
	idx := 0
	for idx < len(lines) && strings.TrimSpace(lines[idx]) == "" {
		idx++
	}
	for _, expected := range header {
		if idx >= len(lines) {
			return lines, false
		}
		if strings.TrimSpace(lines[idx]) != expected {
			return lines, false
		}
		idx++
	}
	for idx < len(lines) && strings.TrimSpace(lines[idx]) == "" {
		idx++
	}
	return lines[idx:], true
}

func matchAndRemoveFooter(lines []string, footer []string) ([]string, bool) {
	if len(footer) == 0 {
		return lines, false
	}
	idx := len(lines) - 1
	for idx >= 0 && strings.TrimSpace(lines[idx]) == "" {
		idx--
	}
	for i := len(footer) - 1; i >= 0; i-- {
		if idx < 0 {
			return lines, false
		}
		if strings.TrimSpace(lines[idx]) != footer[i] {
			return lines, false
		}
		idx--
	}
	for idx >= 0 && strings.TrimSpace(lines[idx]) == "" {
		idx--
	}
	return lines[:idx+1], true
}

func trimEmptyEdges(lines []string) []string {
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	end := len(lines) - 1
	for end >= start && strings.TrimSpace(lines[end]) == "" {
		end--
	}
	if start > end {
		return nil
	}
	return append([]string(nil), lines[start:end+1]...)
}
