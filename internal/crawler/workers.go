package crawler

import "context"

func (c *crawler) internalWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-c.internalJobs:
			if !ok {
				return
			}
			func() {
				defer c.internalWG.Done()
				c.processInternal(ctx, job)
			}()
		}
	}
}

func (c *crawler) externalWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-c.externalJobs:
			if !ok {
				return
			}
			func() {
				defer c.externalWG.Done()
				c.processExternal(ctx, job)
			}()
		}
	}
}
