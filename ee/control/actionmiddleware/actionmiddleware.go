package actionmiddleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/agent/storage/inmemory"
	"github.com/kolide/launcher/pkg/agent/types"
)

const (
	actionRetentionPeriod  = time.Hour * 24 * 7 // a week
	defaultCleanupInterval = time.Hour * 12
)

type updater interface {
	Update(data io.Reader) error
}

type action struct {
	ID          string    `json:"id"`
	ValidUntil  int64     `json:"valid_until"` // timestamp
	Type        string    `json:"type"`
	ProcessedAt time.Time `json:"processed_at,omitempty"`
}

type ActionMiddleware struct {
	ctx                   context.Context
	updaters              map[string]updater
	store                 types.KVStore
	logger                log.Logger
	actionRetentionPeriod time.Duration
	actionCleanupInterval time.Duration
	cancel                context.CancelFunc
}

type actionMiddlewareOption func(*ActionMiddleware)

func WithLogger(logger log.Logger) actionMiddlewareOption {
	return func(a *ActionMiddleware) {
		a.logger = logger
	}
}

func WithActionRetentionPeriod(ttl time.Duration) actionMiddlewareOption {
	return func(a *ActionMiddleware) {
		a.actionRetentionPeriod = ttl
	}
}

func WithStore(store types.KVStore) actionMiddlewareOption {
	return func(a *ActionMiddleware) {
		a.store = store
	}
}

func WithCleanupInterval(cleanupInterval time.Duration) actionMiddlewareOption {
	return func(a *ActionMiddleware) {
		a.actionCleanupInterval = cleanupInterval
	}
}

func WithContext(ctx context.Context) actionMiddlewareOption {
	return func(a *ActionMiddleware) {
		a.ctx = ctx
	}
}

func WithUpdater(updaterType string, updater updater) actionMiddlewareOption {
	return func(a *ActionMiddleware) {
		a.RegisterUpdater(updaterType, updater)
	}
}

func New(opts ...actionMiddlewareOption) *ActionMiddleware {
	amw := &ActionMiddleware{
		ctx:                   context.Background(),
		updaters:              make(map[string]updater, 0),
		actionRetentionPeriod: actionRetentionPeriod,
		actionCleanupInterval: defaultCleanupInterval,
		logger:                log.NewNopLogger(),
	}

	for _, opt := range opts {
		opt(amw)
	}

	if amw.store == nil {
		amw.store = inmemory.NewStore(amw.logger)
	}

	return amw
}

func (a *ActionMiddleware) Update(data io.Reader) error {
	// We want to unmarshal each action separately, so that we don't fail to send all actions
	// if only some are malformed.
	var rawActionsToProcess []json.RawMessage
	if err := json.NewDecoder(data).Decode(&rawActionsToProcess); err != nil {
		return fmt.Errorf("failed to decode actions data: %w", err)
	}

	for _, rawAction := range rawActionsToProcess {
		var action action
		if err := json.Unmarshal(rawAction, &action); err != nil {
			level.Debug(a.logger).Log("msg", "received action in unexpected format from K2, discarding", "err", err)
			continue
		}

		if !a.isActionValid(action) || !a.isActionNew(action.ID) {
			continue
		}

		updater, err := a.updaterForAction(action)
		if err != nil {
			level.Info(a.logger).Log("msg", "getting updater for action", "error", err)
			continue
		}

		if err := updater.Update(bytes.NewReader(rawAction)); err != nil {
			level.Info(a.logger).Log("msg", "failed to update with action, not marking action complete", "err", err)
			continue
		}

		// only mark processed when updater was successful
		action.ProcessedAt = time.Now()
		a.storeActionRecord(action)
	}

	return nil
}

func (a *ActionMiddleware) RegisterUpdater(updaterType string, updater updater) {
	a.updaters[updaterType] = updater
}

func (a *ActionMiddleware) StartCleanup() error {
	a.runCleanup()
	return nil
}

func (a *ActionMiddleware) runCleanup() {
	ctx, cancel := context.WithCancel(a.ctx)
	a.cancel = cancel

	t := time.NewTicker(a.actionCleanupInterval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			level.Debug(a.logger).Log("msg", "action cleanup stopped due to context cancel")
			return
		case <-t.C:
			a.cleanupActions()
		}
	}
}

func (a *ActionMiddleware) StopCleanup(err error) {
	a.cancel()
}

func (a *ActionMiddleware) storeActionRecord(action action) {
	rawAction, err := json.Marshal(action)
	if err != nil {
		level.Error(a.logger).Log("msg", "could not marshal complete action", "err", err)
		return
	}

	if err := a.store.Set([]byte(action.ID), rawAction); err != nil {
		level.Debug(a.logger).Log("msg", "could not mark notification sent", "err", err)
	}
}

func (a *ActionMiddleware) isActionNew(id string) bool {
	sentNotificationRaw, err := a.store.Get([]byte(id))
	if err != nil {
		level.Error(a.logger).Log("msg", "could not read action from bucket", "err", err)
	}

	if sentNotificationRaw == nil {
		// No previous record -- action has not been processed before, it's new
		return true
	}

	return false
}

func (amw *ActionMiddleware) isActionValid(a action) bool {
	if a.ID == "" {
		level.Info(amw.logger).Log("msg", "action ID is empty", "action", a)
		return false
	}

	if a.ValidUntil <= 0 {
		level.Info(amw.logger).Log("msg", "action valid until is empty", "action", a)
		return false
	}

	return a.ValidUntil > time.Now().Unix()
}

func (amw *ActionMiddleware) updaterForAction(a action) (updater, error) {
	if len(amw.updaters) == 0 {
		return nil, errors.New("no updaters registered")
	}

	if len(amw.updaters) == 1 {
		// if we only have one updater, just get that from the map
		// this is a bit of a hack while we transition to using
		// actions
		for _, v := range amw.updaters {
			return v, nil
		}
	}

	// more than one updater
	if a.Type == "" {
		return nil, errors.New("have more than 1 updater and action type is empty, cannot determine updater")
	}

	updater, ok := amw.updaters[a.Type]
	if !ok {
		return nil, fmt.Errorf("updater type %s not found", a.Type)
	}

	return updater, nil
}

func (a *ActionMiddleware) cleanupActions() {
	// Read through all keys in bucket to determine which ones are old enough to be deleted
	keysToDelete := make([][]byte, 0)

	if err := a.store.ForEach(func(k, v []byte) error {
		var processedAction action
		if err := json.Unmarshal(v, &processedAction); err != nil {
			return fmt.Errorf("error processing %s: %w", string(k), err)
		}

		if processedAction.ProcessedAt.Add(a.actionRetentionPeriod).Before(time.Now()) {
			keysToDelete = append(keysToDelete, k)
		}

		return nil
	}); err != nil {
		level.Debug(a.logger).Log("msg", "could not iterate over bucket items to determine which are expired", "err", err)
	}

	// Delete all old keys
	if err := a.store.Delete(keysToDelete...); err != nil {
		level.Debug(a.logger).Log("msg", "could not delete old notifications from bucket", "err", err)
	}
}
