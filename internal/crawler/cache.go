package crawler

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type cacheData struct {
	Visited map[string]cacheEntry `json:"visited"`
}

type cacheEntry struct {
	URL         string    `json:"url"`
	Status      int       `json:"status"`
	Error       string    `json:"error,omitempty"`
	LastVisited time.Time `json:"lastVisited"`
}

func (c *crawler) isCached(pageURL string) bool {
	if c.cachePath == "" {
		return false
	}
	c.cacheMu.RLock()
	_, ok := c.cache.Visited[pageURL]
	c.cacheMu.RUnlock()
	return ok
}

func (c *crawler) updateCache(page *PageReport, visitedAt time.Time) {
	if c.cachePath == "" || page == nil || page.URL == "" {
		return
	}
	c.cacheMu.Lock()
	if c.cache.Visited == nil {
		c.cache.Visited = make(map[string]cacheEntry)
	}
	c.cache.Visited[page.URL] = cacheEntry{
		URL:         page.URL,
		Status:      page.Status,
		Error:       page.Error,
		LastVisited: visitedAt.UTC(),
	}
	c.cacheMu.Unlock()
}

func (c *crawler) writeCache() error {
	if c.cachePath == "" {
		return nil
	}
	c.cacheMu.RLock()
	copyVisited := make(map[string]cacheEntry, len(c.cache.Visited))
	for k, v := range c.cache.Visited {
		copyVisited[k] = v
	}
	c.cacheMu.RUnlock()

	data := cacheData{Visited: copyVisited}
	payload, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(c.cachePath)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(c.cachePath, payload, 0o644)
}

func loadCache(path string) (cacheData, error) {
	data := cacheData{Visited: make(map[string]cacheEntry)}
	if path == "" {
		return data, nil
	}
	payload, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return data, nil
	}
	if err != nil {
		return data, err
	}
	if len(payload) == 0 {
		return data, nil
	}
	if err := json.Unmarshal(payload, &data); err != nil {
		return cacheData{}, err
	}
	if data.Visited == nil {
		data.Visited = make(map[string]cacheEntry)
	}
	return data, nil
}
