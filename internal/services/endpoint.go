package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	v1 "github.com/REDACTED/code/servok/internal/proto/servok/api/v1"
	"github.com/REDACTED/code/servok/internal/sources/srvrecord"
)

func NewEndpointServicer(shutdownCtx context.Context) (v1.EndpointServiceServer, error) {
	es := &endpointServicer{shutdownCtx: shutdownCtx, watchers: map[string]*watcher{}}
	return es, nil
}

type endpointServicer struct {
	v1.UnimplementedEndpointServiceServer
	sync.Mutex

	shutdownCtx context.Context
	watchers    map[string]*watcher
}

func (es *endpointServicer) Watch(request *v1.WatchRequest, stream v1.EndpointService_WatchServer) error {
	// TODO switch on multiple source types
	srvRequest := request.GetSrv()

	qualifiedName := fmt.Sprintf("_%s._%s.%s",
		srvRequest.Service,
		srvRequest.Protocol,
		srvRequest.DnsName,
	)
	log.Info().Str("dnsName", qualifiedName).Msg("client connected")

	updateChannel := make(chan *v1.WatchResponse)
	info := &clientInfo{updateChannel: updateChannel}
	var finalStatus error

	es.Lock()

	// Find a watcher for this dnsName
	watcherForName, ok := es.watchers[qualifiedName]
	if !ok {
		// TODO make the polling period configurable, (or in the request?)
		source, err := srvrecord.NewSrvRecordSource(
			es.shutdownCtx,
			srvRequest.Service,
			srvRequest.Protocol,
			srvRequest.DnsName,
			1*time.Second,
		)
		if err != nil {
			es.Unlock()
			log.Info().Str("dnsName", qualifiedName).Msg("client disconnected")
			return status.Errorf(codes.InvalidArgument, "unable to initialize endpoint source: %s", err)
		}

		// Create the watcher
		watcherForName = &watcher{
			shutdownCtx:  es.shutdownCtx,
			lastResponse: &v1.WatchResponse{},
		}
		es.watchers[qualifiedName] = watcherForName

		// We need to be holding the lock before we kick off the watcher to prevent getting
		// messages out of order and having the watcher mutate its client list before we can
		// insert our client.
		watcherForName.Lock()
		go watcherForName.run(source)
	} else {
		// Since this watcher was already established, send the last response
		if err := stream.Send(watcherForName.lastResponse); err != nil {
			log.Info().Err(err).Str("dnsName", qualifiedName).Msg("client disconnected")
			finalStatus = status.Errorf(codes.Canceled, "attempted to write to closed client stream")
		}

		// Here we take the lock to match the invariant that we hold the lock before
		// leaving the if statement.
		watcherForName.Lock()
	}

	watcherForName.clients = append(watcherForName.clients, info)
	watcherForName.Unlock()
	es.Unlock()

	for es.shutdownCtx.Err() == nil && finalStatus == nil {
		select {
		case update, ok := <-updateChannel:
			if !ok {
				finalStatus = status.Errorf(codes.Internal, "attempted to read from closed update channel")
				break
			}
			if err := stream.Send(update); err != nil {
				log.Info().Err(err).Str("dnsName", qualifiedName).Msg("client disconnected")
				finalStatus = status.Errorf(codes.Canceled, "attempted to write to closed client stream")
			}
		case <-stream.Context().Done():
			log.Info().Str("dnsName", qualifiedName).Msg("client disconnected cleanly")
			finalStatus = status.Errorf(codes.Canceled, "client disconnected")
		case <-es.shutdownCtx.Done():
			finalStatus = status.Errorf(codes.Unavailable, "server disconnected")
		}
	}

	info.Lock()
	defer info.Unlock()
	info.finished = true

	return finalStatus
}
