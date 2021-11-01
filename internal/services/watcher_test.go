package services

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	v1 "github.com/authzed/servok/internal/proto/servok/api/v1"
)

func TestMultiplexingAndCleanup(t *testing.T) {
	testCases := []struct {
		name string
		cf   func(context.CancelFunc, chan []*v1.Endpoint)
	}{
		{
			"CancelFunc",
			func(cancel context.CancelFunc, _ chan []*v1.Endpoint) {
				cancel()
			},
		},
		{
			"CloseUpdateChannel",
			func(_ context.CancelFunc, updateChan chan []*v1.Endpoint) {
				close(updateChan)
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			updateChan := make(chan []*v1.Endpoint)
			ctx, cancel := context.WithCancel(context.Background())

			watcher := &watcher{
				shutdownCtx: ctx,
			}

			var clientChans []chan *v1.WatchResponse
			for i := 0; i < 5; i++ {
				clientChan := make(chan *v1.WatchResponse)
				clientChans = append(clientChans, clientChan)
				watcher.clients = append(watcher.clients, &clientInfo{updateChannel: clientChan})
			}

			exited := false
			go func() {
				watcher.run(updateChan)
				exited = true
			}()

			require := require.New(t)
			require.False(exited)

			// Write an update to the update channel and wait for it on the clients
			updateChan <- []*v1.Endpoint{
				{
					Hostname: "test",
					Port:     50051,
					Weight:   1,
				},
			}

			for _, cc := range clientChans {
				require.Eventually(func() bool {
					select {
					case update, ok := <-cc:
						if ok {
							require.Equal("test", update.Endpoints[0].Hostname)
							require.Equal(uint32(50051), update.Endpoints[0].Port)
							require.Equal(uint32(1), update.Endpoints[0].Weight)
							return true
						}
					default:
					}
					return false
				}, 100*time.Millisecond, 1*time.Millisecond)
			}

			// Signal shutdown
			tc.cf(cancel, updateChan)

			require.Eventually(func() bool {
				return exited
			}, 100*time.Millisecond, 1*time.Millisecond)

			for _, cc := range clientChans {
				select {
				case <-cc:
				default:
					require.Fail("all channels should be closed")
				}
			}
		})
	}
}
