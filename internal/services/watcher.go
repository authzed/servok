package services

import (
	"context"
	"sync"

	v1 "github.com/REDACTED/code/servok/internal/proto/servok/api/v1"
	"github.com/REDACTED/code/servok/internal/sources"
	"github.com/rs/zerolog/log"
)

type clientInfo struct {
	sync.Mutex

	updateChannel chan<- *v1.WatchResponse
	finished      bool
}

type watcher struct {
	sync.Mutex

	shutdownCtx  context.Context
	clients      []*clientInfo
	lastResponse *v1.WatchResponse
}

func (w *watcher) run(endpointUpdates sources.Endpoint) {
	hadError := false
	for w.shutdownCtx.Err() == nil && !hadError {
		select {
		case update, ok := <-endpointUpdates:
			if !ok {
				log.Error().Msg("unable to read updates from endpoint source")
				hadError = true
				break
			}

			updateResponse := &v1.WatchResponse{Endpoints: update}

			startingClients := len(w.clients)
			stillAlive := make([]*clientInfo, 0, startingClients)

			w.Lock()
			w.lastResponse = updateResponse
			for _, client := range w.clients {
				client.Lock()
				if !client.finished {
					client.updateChannel <- updateResponse
					stillAlive = append(stillAlive, client)
				} else {
					close(client.updateChannel)
				}
				client.Unlock()
			}
			w.clients = stillAlive
			w.Unlock()

			prunedClients := startingClients - len(stillAlive)
			if prunedClients > 0 {
				log.Info().Int("pruned", prunedClients).Msg("pruned finished clients")
			}
		case <-w.shutdownCtx.Done():
			log.Info().Msg("shutting down watcher")
		}
	}

	log.Info().Msg("closing client update channels")
	for _, client := range w.clients {
		client.Lock()
		close(client.updateChannel)
		client.Unlock()
	}
}
