package srvrecord

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/proto"

	v1 "github.com/REDACTED/code/servok/internal/proto/servok/api/v1"
	"github.com/REDACTED/code/servok/internal/sources"
)

type resolverFunc func() ([]*net.SRV, error)

func NewSrvRecordSource(shutdownCtx context.Context, service, proto, name string, updatePeriod time.Duration) (sources.Endpoint, error) {
	resolver := func() ([]*net.SRV, error) {
		_, addrs, err := net.LookupSRV(service, proto, name)
		return addrs, err
	}

	_, err := resolver()
	if err != nil {
		return nil, err
	}

	updateChan := make(chan []*v1.Endpoint)

	log.Info().Stringer("period", updatePeriod).Str("service", service).Str("proto", proto).Str("name", name).Msg("starting DNS SRV endpoint source")
	go run(shutdownCtx, updateChan, resolver, updatePeriod)

	return updateChan, nil
}

func run(ctx context.Context,
	updates chan<- []*v1.Endpoint,
	resolver resolverFunc,
	updatePeriod time.Duration) {

	defer close(updates)

	ticker := time.NewTicker(updatePeriod)

	stop := false
	last := &v1.WatchResponse{Endpoints: []*v1.Endpoint{{Hostname: "bootstrap"}}}
	for !stop {
		select {
		case <-ctx.Done():
			stop = true
		case <-ticker.C:
			addrs, err := resolver()
			if err != nil {
				log.Error().Err(err).Msg("error resolving DNS SRV endpoints")
				stop = true
				break
			}
			endpoints := rewriteAndSortAddrs(addrs)

			next := &v1.WatchResponse{Endpoints: endpoints}

			if !proto.Equal(last, next) {
				numEntries := len(endpoints)
				log.Debug().Int("numEntries", numEntries).Msg("writing DNS SRV updates to the channel")
				updates <- endpoints
				log.Debug().Int("numEntries", numEntries).Msg("DNS SRV updates written")
			}
			last = next
		}
	}

	log.Info().Msg("stopping DNS SRV endpoint source")
}

func rewriteAndSortAddrs(addrs []*net.SRV) []*v1.Endpoint {
	var resolved []*v1.Endpoint
	for _, addr := range addrs {
		endpoint := &v1.Endpoint{
			Hostname: addr.Target,
			Port:     uint32(addr.Port),
			Weight:   uint32(addr.Weight),
		}

		resolved = append(resolved, endpoint)
	}

	sort.Slice(resolved, func(li, ri int) bool {
		left, right := canonicalSRV(resolved[li]), canonicalSRV(resolved[ri])
		return strings.Compare(left, right) < 0
	})

	return resolved
}

func canonicalSRV(endpoint *v1.Endpoint) string {
	return fmt.Sprintf("0 %d %d %s", endpoint.Weight, endpoint.Port, endpoint.Hostname)
}
