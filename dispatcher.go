package pinqloq

import (
	"context"
	"log"
	"sync"
	"time"
)

type batchSender interface {
	sendBatch(ctx context.Context, items []queuedLog)
}

type logDispatcher struct {
	buffer    *logBuffer
	apiClient batchSender
	options   Options
	wg        sync.WaitGroup
	started   bool
	mu        sync.Mutex
}

func newLogDispatcher(buffer *logBuffer, apiClient batchSender, options Options) *logDispatcher {
	return &logDispatcher{buffer: buffer, apiClient: apiClient, options: options}
}

func (d *logDispatcher) start() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.started {
		return
	}
	d.started = true

	d.wg.Add(1)
	go d.run()
}

func (d *logDispatcher) run() {
	defer d.wg.Done()

	ch := d.buffer.channel()
	batch := make([]queuedLog, 0, d.options.BatchSize)

	for {
		first, ok := <-ch
		if !ok {
			return
		}

		batch = batch[:0]
		batch = append(batch, first)

		timer := time.NewTimer(d.options.FlushInterval)
	collect:
		for len(batch) < d.options.BatchSize {
			select {
			case item, ok := <-ch:
				if !ok {
					break collect
				}
				batch = append(batch, item)
			case <-timer.C:
				break collect
			}
		}
		timer.Stop()

		d.sendBatch(batch)
	}
}

func (d *logDispatcher) sendBatch(batch []queuedLog) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("pinqloq: batch of %d logs could not be sent; dropped. %v", len(batch), r)
		}
	}()

	d.apiClient.sendBatch(context.Background(), batch)
}

func (d *logDispatcher) shutdown(ctx context.Context) error {
	d.buffer.close()

	done := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
