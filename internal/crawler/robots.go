package crawler

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type robotsGroup struct {
	allows    []string
	disallows []string
}

func (c *crawler) allowedByRobots(ctx context.Context, u *url.URL) bool {
	if c.ignoreRobots {
		return true
	}
	host := strings.ToLower(u.Host)
	if host == "" {
		return true
	}

	c.robotsMu.Lock()
	group, ok := c.robots[host]
	c.robotsMu.Unlock()
	if !ok {
		group = c.fetchRobots(ctx, u)
		c.robotsMu.Lock()
		c.robots[host] = group
		c.robotsMu.Unlock()
	}
	if group == nil {
		return true
	}

	pathValue := u.EscapedPath()
	if pathValue == "" {
		pathValue = "/"
	}
	return group.Allowed(pathValue)
}

func (c *crawler) fetchRobots(ctx context.Context, u *url.URL) *robotsGroup {
	robotsURL := &url.URL{
		Scheme: u.Scheme,
		Host:   u.Host,
		Path:   "/robots.txt",
	}
	if !c.acquireRequestSlot(ctx) {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, robotsURL.String(), nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", defaultUserAgent)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return nil
	}
	return parseRobots(body, defaultUserAgent)
}

func parseRobots(payload []byte, userAgent string) *robotsGroup {
	scanner := bufio.NewScanner(bytes.NewReader(payload))
	groups := make(map[string]*robotsGroup)
	var currentAgents []string
	hadDirective := false

	for scanner.Scan() {
		line := scanner.Text()
		if idx := strings.Index(line, "#"); idx >= 0 {
			line = line[:idx]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			currentAgents = nil
			hadDirective = false
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])

		switch key {
		case "user-agent":
			if hadDirective {
				currentAgents = nil
				hadDirective = false
			}
			agent := strings.ToLower(value)
			currentAgents = append(currentAgents, agent)
		case "allow", "disallow":
			if len(currentAgents) == 0 {
				continue
			}
			rule := sanitizeRobotsRule(value)
			if rule == "" && key == "disallow" {
				continue
			}
			hadDirective = true
			for _, agent := range currentAgents {
				group := groups[agent]
				if group == nil {
					group = &robotsGroup{}
					groups[agent] = group
				}
				if key == "allow" {
					group.allows = append(group.allows, rule)
				} else {
					group.disallows = append(group.disallows, rule)
				}
			}
		}
	}

	baseAgent := strings.ToLower(strings.Split(userAgent, "/")[0])
	if group, ok := groups[baseAgent]; ok {
		return group
	}
	if group, ok := groups[userAgent]; ok {
		return group
	}
	if group, ok := groups["*"]; ok {
		return group
	}
	return nil
}

func sanitizeRobotsRule(rule string) string {
	cleaned := strings.TrimSpace(rule)
	if cleaned == "" {
		return ""
	}
	cleaned = strings.ReplaceAll(cleaned, "*", "")
	if !strings.HasPrefix(cleaned, "/") {
		cleaned = "/" + cleaned
	}
	return cleaned
}

func (rg *robotsGroup) Allowed(pathValue string) bool {
	if rg == nil {
		return true
	}
	allowMatch := matchLongestPrefix(pathValue, rg.allows)
	disallowMatch := matchLongestPrefix(pathValue, rg.disallows)

	if len(disallowMatch) == 0 {
		return true
	}
	if len(allowMatch) > len(disallowMatch) {
		return true
	}
	if len(allowMatch) == len(disallowMatch) && len(allowMatch) > 0 {
		return true
	}
	return false
}

func matchLongestPrefix(pathValue string, rules []string) string {
	longest := ""
	for _, rule := range rules {
		if rule == "" {
			continue
		}
		if strings.HasPrefix(pathValue, rule) && len(rule) > len(longest) {
			longest = rule
		}
	}
	return longest
}
