package updater

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/cmd/launcher/internal/updater/mocks"
	"github.com/kolide/updater/tuf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func Test_updaterCmd_execute(t *testing.T) {
	t.Parallel()

	type fields struct {
		// Mock generated with `mockery --name updater --exported`
		updater                 *mocks.Updater
		stopChan                chan bool
		config                  *UpdaterConfig
		runUpdaterRetryInterval time.Duration
	}
	tests := []struct {
		name   string
		fields fields
		// in this test, the calls to run are the only thing we can really assert against
		// leave this field empty and the test will fail if there is a call to run function made
		// add 3 funcs here and the test will expect updater.Run() to be called 3 times
		updaterRunReturns []func(opts ...tuf.Option) (stop func(), err error)
		callStopChanAfter time.Duration
		assertion         assert.ErrorAssertionFunc
	}{
		{
			name: "success",
			fields: fields{
				updater: &mocks.Updater{},
				config: &UpdaterConfig{
					Logger: log.NewNopLogger(),
				},
			},
			updaterRunReturns: []func(opts ...tuf.Option) (stop func(), err error){
				func(opts ...tuf.Option) (stop func(), err error) {
					return func() {}, nil
				},
			},
			assertion: assert.NoError,
		},
		{
			name: "multiple_run_retries",
			fields: fields{
				updater: &mocks.Updater{},
				config: &UpdaterConfig{
					Logger: log.NewNopLogger(),
				},
				runUpdaterRetryInterval: time.Millisecond,
			},
			updaterRunReturns: []func(opts ...tuf.Option) (stop func(), err error){
				func(opts ...tuf.Option) (stop func(), err error) {
					return nil, errors.New("some error")
				},
				func(opts ...tuf.Option) (stop func(), err error) {
					return nil, errors.New("some error")
				},
				func(opts ...tuf.Option) (stop func(), err error) {
					return func() {}, nil
				},
			},
			assertion: assert.NoError,
		},
		{
			name: "stop_during_initial_delay",
			fields: fields{
				updater:  &mocks.Updater{},
				stopChan: make(chan bool),
				config: &UpdaterConfig{
					Logger:       log.NewNopLogger(),
					InitialDelay: 200 * time.Millisecond,
				},
			},
			callStopChanAfter: time.Millisecond,
			assertion:         assert.NoError,
		},
		{
			name: "stop_during_retry_loop",
			fields: fields{
				updater:  &mocks.Updater{},
				stopChan: make(chan bool),
				config: &UpdaterConfig{
					Logger: log.NewNopLogger(),
				},
				runUpdaterRetryInterval: 1 * time.Second,
			},
			updaterRunReturns: []func(opts ...tuf.Option) (stop func(), err error){
				func(opts ...tuf.Option) (stop func(), err error) {
					return nil, errors.New("some error")
				},
			},
			callStopChanAfter: 5 * time.Millisecond,
			assertion:         assert.NoError,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancelCtx := context.WithTimeout(context.Background(), 0)
			defer cancelCtx()

			u := &updaterCmd{
				updater:                 tt.fields.updater,
				ctx:                     ctx,
				stopChan:                tt.fields.stopChan,
				config:                  tt.fields.config,
				runUpdaterRetryInterval: tt.fields.runUpdaterRetryInterval,
			}

			var wg sync.WaitGroup
			if tt.callStopChanAfter > 0 {
				wg.Add(1)
				go func() {
					time.Sleep(tt.callStopChanAfter)
					tt.fields.stopChan <- true
					wg.Done()
				}()
			}

			if tt.updaterRunReturns != nil {
				for _, returnFunc := range tt.updaterRunReturns {
					tt.fields.updater.On("Run", mock.AnythingOfType("tuf.Option"), mock.AnythingOfType("tuf.Option")).Return(returnFunc()).Once()
				}
			}

			tt.assertion(t, u.execute())
			tt.fields.updater.AssertExpectations(t)

			// test will time out if we don't get to send something on u.stopChan when expecting channel receive
			wg.Wait()
		})
	}
}

func Test_updaterCmd_interrupt(t *testing.T) {
	t.Parallel()

	type fields struct {
		stopChan chan bool
		config   *UpdaterConfig
	}
	type args struct {
		err error
	}
	tests := []struct {
		name                     string
		fields                   fields
		args                     args
		expectStopChannelReceive bool
		expectedCallsToStop      int
	}{
		{
			name: "default_interrupt",
			fields: fields{
				stopChan: make(chan bool),
				config: &UpdaterConfig{
					Logger: log.NewNopLogger(),
				},
			},
			args: args{
				err: errors.New("some error"),
			},
			expectedCallsToStop: 1,
		},
		{
			name: "channel_send_interrupt",
			fields: fields{
				stopChan: make(chan bool),
				config: &UpdaterConfig{
					Logger: log.NewNopLogger(),
				},
			},
			args: args{
				err: errors.New("some error"),
			},
			expectedCallsToStop:      1,
			expectStopChannelReceive: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			u := &updaterCmd{
				stopChan: tt.fields.stopChan,
				config:   tt.fields.config,
			}

			// using this wait group to ensure that something gets received on u.StopChan
			// wonder if there is a more elegant way
			var wg sync.WaitGroup
			if tt.expectStopChannelReceive {
				wg.Add(1)
				go func() {
					<-u.stopChan
					wg.Done()
				}()
				time.Sleep(5 * time.Millisecond)
			}

			stopCalledCount := 0
			stopFunc := func() {
				stopCalledCount++
			}
			u.stopExecution = stopFunc

			u.interrupt(tt.args.err)
			assert.Equal(t, tt.expectedCallsToStop, stopCalledCount)

			// test will time out if we don't get something on u.stopChan when expecting channel receive
			wg.Wait()
		})
	}
}
