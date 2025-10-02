package crawler

import (
	"net/url"
	"path"
	"strings"
)

func (c *crawler) normalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if !parsed.IsAbs() {
		parsed = c.start.ResolveReference(parsed)
	}
	if parsed.Scheme == "" {
		parsed.Scheme = c.start.Scheme
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	if parsed.Host == "" {
		parsed.Host = c.start.Host
	}

	normalized := *parsed
	normalized.Fragment = ""
	normalized.Scheme = strings.ToLower(normalized.Scheme)
	normalized.Host = strings.ToLower(normalized.Host)

	origPath := parsed.Path
	cleaned := path.Clean(origPath)
	if cleaned == "." || cleaned == "" {
		cleaned = "/"
	}
	if strings.HasSuffix(origPath, "/") && cleaned != "/" {
		cleaned += "/"
	}
	normalized.Path = cleaned

	return normalized.String()
}
