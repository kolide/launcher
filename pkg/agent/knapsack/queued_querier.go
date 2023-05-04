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

type synchronousQuerier interface {
	Query(query string) ([]map[string]string, error)
}

type queuedQuerier struct {
	logger      log.Logger
	cancel      context.CancelFunc
	queueCh     chan struct{}
	queue       *queue
	syncQuerier synchronousQuerier
}

type queue struct {
	items *list.List
	mutex sync.Mutex
}

func (q *queue) push(item *queueItem) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	q.items.PushBack(item)
}

func (q *queue) pop() *queueItem {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	e := q.items.Front() // First element
	if e == nil {
		return nil
	}

	q.items.Remove(e) // Dequeue

	if e == nil {
		return nil
	}

	item, ok := e.Value.(*queueItem)
	if ok {
		return item
	}

	return nil
}

type queueItem struct {
	query    string
	callback func(result []map[string]string, err error)
}

func NewQueuedQuerier(logger log.Logger) *queuedQuerier {
	q := &queue{
		items: list.New(),
	}
	qq := &queuedQuerier{
		logger:  log.With(logger, "component", "querier"),
		queue:   q,
		queueCh: make(chan struct{}, 10),
	}

	return qq
}

func (qq *queuedQuerier) SetQuerier(querier synchronousQuerier) {
	level.Info(qq.logger).Log("msg", "queued querier has synchronous querier")
	qq.syncQuerier = querier

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
		if qq.syncQuerier != nil {
			for {
				// Pop items off the queue until it's empty
				item := qq.queue.pop()
				if item == nil {
					break
				}
				// Try to run the query
				result, err := queryWithRetries(qq.syncQuerier, item.query)
				// Give the error and/or results back to the client via the callback
				item.callback(result, err)
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-qq.queueCh:
			// Go process the queue
			continue
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

func (qq *queuedQuerier) Query(query string, callback func(result []map[string]string, err error)) error {
	if qq == nil {
		return errors.New("queuedQuerier is nil")
	}

	// Push the item on to the queue, and notify the channel so the query is processed
	qq.queue.push(&queueItem{query: query, callback: callback})
	qq.queueCh <- struct{}{}

	return nil
}

func queryWithRetries(querier synchronousQuerier, query string) ([]map[string]string, error) {
	var results []map[string]string
	var err error

	backoff.WaitFor(func() error {
		results, err = querier.Query(query)
		return err
	}, 1*time.Second, 250*time.Millisecond)

	return results, err
}
