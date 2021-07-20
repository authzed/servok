package services

import (
	"context"
	"sync"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	v1 "github.com/REDACTED/code/servok/api/v1"
	"github.com/REDACTED/code/servok/internal/sources"
)

func NewEndpointServicer(shutdownCtx context.Context, endpointUpdates sources.Endpoint) (v1.EndpointServiceServer, error) {
	es := &endpointServicer{shutdownCtx: shutdownCtx}
	go es.run(endpointUpdates)
	return es, nil
}

type endpointServicer struct {
	v1.UnimplementedEndpointServiceServer
	sync.Mutex

	shutdownCtx  context.Context
	clients      []*clientInfo
	lastResponse *v1.WatchResponse
}

type clientInfo struct {
	sync.Mutex

	updateChannel chan<- *v1.WatchResponse
	finished      bool
}

type EndpointSource <-chan []*v1.Endpoint

func (es *endpointServicer) run(endpointUpdates sources.Endpoint) {
	hadError := false
	for es.shutdownCtx.Err() == nil && !hadError {
		select {
		case update, ok := <-endpointUpdates:
			if !ok {
				log.Error().Msg("unable to read updates from endpoint source")
				hadError = true
				break
			}

			updateResponse := &v1.WatchResponse{Endpoints: update}

			startingClients := len(es.clients)
			stillAlive := make([]*clientInfo, 0, startingClients)

			es.Lock()
			es.lastResponse = updateResponse
			for _, client := range es.clients {
				client.Lock()
				if !client.finished {
					client.updateChannel <- updateResponse
					stillAlive = append(stillAlive, client)
				} else {
					close(client.updateChannel)
				}
				client.Unlock()
			}
			es.clients = stillAlive
			es.Unlock()

			prunedClients := startingClients - len(stillAlive)
			if prunedClients > 0 {
				log.Info().Int("pruned", prunedClients).Msg("pruned finished clients")
			}
		case <-es.shutdownCtx.Done():
			log.Info().Msg("shutting down endpoint service")
		}
	}

	log.Info().Msg("closing client update channels")
	for _, client := range es.clients {
		client.Lock()
		close(client.updateChannel)
		client.Unlock()
	}
}

func (es *endpointServicer) Watch(request *v1.WatchRequest, stream v1.EndpointService_WatchServer) error {
	log.Info().Msg("client connected")

	updateChannel := make(chan *v1.WatchResponse)
	info := &clientInfo{updateChannel: updateChannel}
	var finalStatus error

	es.Lock()
	if err := stream.Send(es.lastResponse); err != nil {
		log.Info().Err(err).Msg("client disconnected")
		finalStatus = status.Errorf(codes.Canceled, "attempted to write to closed client stream")
	}
	es.clients = append(es.clients, info)
	es.Unlock()

	for es.shutdownCtx.Err() == nil && finalStatus == nil {
		select {
		case update, ok := <-updateChannel:
			if !ok {
				finalStatus = status.Errorf(codes.Internal, "attempted to read from closed update channel")
			}
			if err := stream.Send(update); err != nil {
				log.Info().Err(err).Msg("client disconnected")
				finalStatus = status.Errorf(codes.Canceled, "attempted to write to closed client stream")
			}
		case <-stream.Context().Done():
			log.Info().Msg("client disconnected cleanly")
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
