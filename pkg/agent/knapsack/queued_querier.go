package knapsack

import (
	"container/list"
	"context"
	"errors"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/backoff"
)

// querier is an interface for synchronously querying osquery.
type Querier interface {
	Query(query string) ([]map[string]string, error)
}

// queuedQuerier maintains a queue of queries to be processed asynchronously
// as queries are added to the queue.
type queuedQuerier struct {
	logger  log.Logger
	cancel  context.CancelFunc
	queueCh chan struct{}
	queue   *list.List
	mutex   sync.Mutex
	querier Querier
}

// queueItem is encapsulates everything that the queuedQuerier needs to process a query and return the error/result.
type queueItem struct {
	query    string
	callback func(result []map[string]string, err error)
}

func NewQueuedQuerier(logger log.Logger) *queuedQuerier {
	qq := &queuedQuerier{
		logger:  log.With(logger, "component", "querier"),
		queueCh: make(chan struct{}, 10),
		queue:   list.New(),
	}

	return qq
}

// SetQuerier provides the underlying synchronous version of querier. If there are queries in
// the queue pending processing, those will start processing now.
func (qq *queuedQuerier) SetQuerier(querier Querier) {
	level.Info(qq.logger).Log("msg", "setting querier for queued querier")
	qq.querier = querier

	// Wake up in case any queries were received prior to having the synchronous querier
	qq.queueCh <- struct{}{}
}

// ExecuteWithContext returns an Execute function suitable for rungroup. It's a
// wrapper over the Start function, which takes a context.Context.
func (qq *queuedQuerier) ExecuteWithContext(ctx context.Context) func() error {
	return func() error {
		qq.Start(ctx)
		return nil
	}
}

func (qq *queuedQuerier) Start(ctx context.Context) {
	level.Info(qq.logger).Log("msg", "queued querier started")
	ctx, qq.cancel = context.WithCancel(ctx)

	for {
		qq.processQueue()

		select {
		case <-ctx.Done():
			return
		case <-qq.queueCh:
			// Go process the queue
			continue
		}
	}
}

func (qq *queuedQuerier) processQueue() {
	if qq.querier != nil {
		for {
			// Pop items off the queue until it's empty
			item := qq.pop()
			if item == nil {
				break
			}
			// Try to run the query
			result, err := queryWithRetries(qq.querier, item.query)
			// Give the error and/or results back to the client via the callback
			item.callback(result, err)
		}
	}
}

func (qq *queuedQuerier) Interrupt(err error) {
	level.Info(qq.logger).Log("msg", "queued querier interrupted", "err", err)
	qq.Stop()
}

func (qq *queuedQuerier) Stop() {
	level.Info(qq.logger).Log("msg", "queued querier stopping")
	close(qq.queueCh)
	if qq.cancel != nil {
		qq.cancel()
	}
}

// Query will attempt to send a query to the osquery client. The result of the
// query will be passed to the callback function provided.
func (qq *queuedQuerier) Query(query string, callback func(result []map[string]string, err error)) error {
	if qq == nil {
		return errors.New("queuedQuerier is nil")
	}

	// Push the item on to the queue, and notify the channel so the query is processed
	qq.push(&queueItem{query: query, callback: callback})
	qq.queueCh <- struct{}{}

	return nil
}

// push locks the queue and inserts an item at the back of the queue
func (qq *queuedQuerier) push(item *queueItem) {
	qq.mutex.Lock()
	defer qq.mutex.Unlock()

	qq.queue.PushBack(item)
}

// pop locks the queue, removes and returns the first item of the queue
func (qq *queuedQuerier) pop() *queueItem {
	qq.mutex.Lock()
	defer qq.mutex.Unlock()

	e := qq.queue.Front() // First element
	if e == nil {
		return nil
	}

	qq.queue.Remove(e) // Dequeue

	if e == nil {
		return nil
	}

	item, ok := e.Value.(*queueItem)
	if ok {
		return item
	}

	return nil
}

// queryWithRetries attempts to run the query, retrying for a perod of time, as necessary
func queryWithRetries(querier Querier, query string) ([]map[string]string, error) {
	var results []map[string]string
	var err error

	backoff.WaitFor(func() error {
		results, err = querier.Query(query)
		return err
	}, 1*time.Second, 250*time.Millisecond)

	return results, err
}
