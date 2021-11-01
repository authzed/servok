package srvrecord

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"

	v1 "github.com/authzed/servok/internal/proto/servok/api/v1"
)

func TestRewriteAddrs(t *testing.T) {
	testCases := []struct {
		name     string
		addrs    []*net.SRV
		expected []*v1.Endpoint
	}{
		{
			"single addr",
			[]*net.SRV{
				{Target: "host1", Port: 50051, Priority: 0, Weight: 1},
			},
			[]*v1.Endpoint{
				{Hostname: "host1", Port: 50051, Weight: 1},
			},
		},
		{
			"multiple addrs",
			[]*net.SRV{
				{Target: "host1", Port: 50051, Priority: 0, Weight: 1},
				{Target: "host2", Port: 50051, Priority: 0, Weight: 1},
			},
			[]*v1.Endpoint{
				{Hostname: "host1", Port: 50051, Weight: 1},
				{Hostname: "host2", Port: 50051, Weight: 1},
			},
		},
		{
			"sort order",
			[]*net.SRV{
				{Target: "host2", Port: 50051, Priority: 0, Weight: 5},
				{Target: "host2", Port: 50051, Priority: 0, Weight: 1},
				{Target: "host1", Port: 50051, Priority: 0, Weight: 1},
				{Target: "host1", Port: 50052, Priority: 0, Weight: 1},
			},
			[]*v1.Endpoint{
				{Hostname: "host1", Port: 50051, Weight: 1},
				{Hostname: "host2", Port: 50051, Weight: 1},
				{Hostname: "host1", Port: 50052, Weight: 1},
				{Hostname: "host2", Port: 50051, Weight: 5},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require := require.New(t)

			rewrittenResponse := &v1.WatchResponse{Endpoints: rewriteAndSortAddrs(tc.addrs)}
			expectedResponse := &v1.WatchResponse{Endpoints: tc.expected}

			require.Empty(cmp.Diff(expectedResponse, rewrittenResponse, protocmp.Transform()))
		})
	}
}

func TestRun(t *testing.T) {
	testCases := []struct {
		name                 string
		resolverAddrs        [][]*net.SRV
		resolverErr          error
		expectedResultCounts []int
	}{
		{"empty resolver response", nil, nil, []int{0}},
		{
			"single entry",
			[][]*net.SRV{
				{
					{Target: "host1", Port: 50051, Priority: 0, Weight: 1},
				},
			},
			nil,
			[]int{1},
		},
		{
			"multiple entries",
			[][]*net.SRV{
				{
					{Target: "host1", Port: 50051, Priority: 0, Weight: 1},
					{Target: "host2", Port: 50051, Priority: 0, Weight: 2},
				},
			},
			nil,
			[]int{2},
		},
		{
			"changing entries",
			[][]*net.SRV{
				{
					{Target: "host1", Port: 50051, Priority: 0, Weight: 1},
					{Target: "host2", Port: 50051, Priority: 0, Weight: 2},
				},
				{
					{Target: "host3", Port: 50051, Priority: 0, Weight: 1},
				},
			},
			nil,
			[]int{2, 1},
		},
		{"resolver error", nil, errors.New("resolver error!"), []int{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require := require.New(t)

			ctx, cancel := context.WithCancel(context.Background())
			updateChan := make(chan []*v1.Endpoint)

			var index int
			fakeResolver := func() ([]*net.SRV, error) {
				if tc.resolverErr != nil {
					return nil, tc.resolverErr
				}

				scriptLen := len(tc.resolverAddrs)
				if scriptLen > 0 {
					if index >= scriptLen {
						return tc.resolverAddrs[scriptLen-1], nil
					}
					index += 1
					return tc.resolverAddrs[index-1], nil
				}
				return nil, nil
			}

			var exited bool
			go func() {
				run(ctx, updateChan, fakeResolver, 500*time.Microsecond)
				exited = true
			}()

			require.False(exited)

			for _, expectedCount := range tc.expectedResultCounts {
				require.Eventually(func() bool {
					select {
					case update, ok := <-updateChan:
						if ok {
							require.Len(update, expectedCount)
							return true
						}
					default:
					}
					return false
				}, 100*time.Millisecond, 1*time.Millisecond)
			}

			if tc.resolverErr != nil {
				update, ok := <-updateChan
				require.False(ok)
				require.Nil(update)
			}

			cancel()

			require.Eventually(func() bool {
				return exited
			}, 100*time.Millisecond, 1*time.Millisecond)

			select {
			case <-updateChan:
			default:
				require.Fail("update channel should be closed")
			}
		})
	}
}
