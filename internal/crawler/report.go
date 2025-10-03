package crawler

func (c *crawler) recordError(err Error) {
	c.reportMu.Lock()
	c.errors = append(c.errors, err)
	c.reportMu.Unlock()
}

func (c *crawler) savePage(page *PageReport) {
    c.reportMu.Lock()
    if existing, ok := c.pages[page.URL]; ok {
        if page.Error != "" {
            existing.Error = page.Error
		}
		if len(page.Links) > 0 {
			existing.Links = page.Links
		}
		if page.Status != 0 {
			existing.Status = page.Status
		}
        if page.Retrieved != 0 {
            existing.Retrieved = page.Retrieved
        }
        if page.MarkdownPath != "" {
            existing.MarkdownPath = page.MarkdownPath
            existing.MarkdownSkippedReason = ""
        } else if page.MarkdownSkippedReason != "" && existing.MarkdownPath == "" {
            existing.MarkdownSkippedReason = page.MarkdownSkippedReason
        }
    } else {
        c.pages[page.URL] = page
    }
    c.reportMu.Unlock()
}
