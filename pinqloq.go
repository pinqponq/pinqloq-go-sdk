package pinqloq

import "context"

// Client bundles a buffered, batched log shipper with an HTTP request-logging middleware factory.
type Client struct {
	logger     Logger
	dispatcher *logDispatcher
	options    Options
}

// New creates a Pinqloq client: a buffered, batched log shipper plus an HTTP middleware factory.
func New(opts Options) (*Client, error) {
	resolved, err := resolveOptions(opts)
	if err != nil {
		return nil, err
	}

	buffer := newLogBuffer(resolved.QueueCapacity)
	apiClient := newIngestAPIClient(resolved)
	dispatcher := newLogDispatcher(buffer, apiClient, resolved)
	logger := &defaultLogger{buffer: buffer, dispatcher: dispatcher, options: resolved}

	dispatcher.start()

	return &Client{logger: logger, dispatcher: dispatcher, options: resolved}, nil
}

// Logger sends structured application events; buffered and delivered in the background.
func (c *Client) Logger() Logger {
	return c.logger
}

// Enqueue is a shortcut for Logger().Enqueue.
func (c *Client) Enqueue(entry LogEntry, onSent OnSent, onFailed OnFailed) (bool, error) {
	return c.logger.Enqueue(entry, onSent, onFailed)
}

// EnqueueMany is a shortcut for Logger().EnqueueMany.
func (c *Client) EnqueueMany(entries []LogEntry, onSent OnSent, onFailed OnFailed) (int, error) {
	return c.logger.EnqueueMany(entries, onSent, onFailed)
}

// Shutdown stops the background dispatcher and sends whatever is left in the queue, waiting up
// to ctx's deadline for delivery to finish.
func (c *Client) Shutdown(ctx context.Context) error {
	return c.dispatcher.shutdown(ctx)
}
