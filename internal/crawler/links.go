package crawler

import (
	"html"
	"net/url"
	"path"
	"regexp"
	"strings"
)

var linkPattern = regexp.MustCompile(`(?i)<a[^>]*?\bhref\s*=\s*("([^"]*)"|'([^']*)'|([^\s"'>]+))`)

func (c *crawler) extractLinks(body []byte, base string) []Link {
	matches := linkPattern.FindAllSubmatch(body, -1)
	if len(matches) == 0 {
		return nil
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return nil
	}

	seen := make(map[string]struct{})
	links := make([]Link, 0, len(matches))

	for _, m := range matches {
		href := ""
		switch {
		case len(m) >= 3 && len(m[2]) > 0:
			href = string(m[2])
		case len(m) >= 4 && len(m[3]) > 0:
			href = string(m[3])
		case len(m) >= 5 && len(m[4]) > 0:
			href = string(m[4])
		}
		href = html.UnescapeString(strings.TrimSpace(href))
		if href == "" {
			continue
		}
		lower := strings.ToLower(href)
		switch {
		case strings.HasPrefix(lower, "javascript:"):
			continue
		case strings.HasPrefix(lower, "mailto:"):
			continue
		case strings.HasPrefix(lower, "tel:"):
			continue
		}
		if href == "#" {
			continue
		}

		candidate, err := url.Parse(href)
		if err != nil {
			continue
		}
		if !candidate.IsAbs() {
			candidate = baseURL.ResolveReference(candidate)
		}
		candidate.Fragment = ""
		normalized := c.normalizeURL(candidate.String())
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}

		linkType := LinkTypeExternal
		if strings.EqualFold(candidate.Host, c.start.Host) {
			linkType = LinkTypeInternal
		}
		links = append(links, Link{URL: normalized, Type: linkType})
	}

	return links
}

func buildAllowedExtensions(list []string) map[string]struct{} {
	allowed := make(map[string]struct{})
	if len(list) == 0 {
		allowed[""] = struct{}{}
		allowed[".html"] = struct{}{}
		allowed[".htm"] = struct{}{}
		return allowed
	}
	for _, item := range list {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			allowed[""] = struct{}{}
			continue
		}
		lowered := strings.ToLower(trimmed)
		if lowered != "/" && !strings.HasPrefix(lowered, ".") {
			lowered = "." + lowered
		}
		allowed[lowered] = struct{}{}
	}
	if _, ok := allowed[""]; !ok {
		allowed[""] = struct{}{}
	}
	return allowed
}

func (c *crawler) allowedExtension(u *url.URL) bool {
	if len(c.allowedExt) == 0 {
		return true
	}
	pathValue := u.Path
	if pathValue == "" || strings.HasSuffix(pathValue, "/") {
		if _, ok := c.allowedExt[""]; ok {
			return true
		}
		_, ok := c.allowedExt["/"]
		return ok
	}
	ext := strings.ToLower(path.Ext(pathValue))
	if ext == "" {
		_, ok := c.allowedExt[""]
		return ok
	}
	_, ok := c.allowedExt[ext]
	return ok
}
