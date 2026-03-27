package main

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/valkey-io/valkey-go"
)

func (app *application) ssestatusHandler(w http.ResponseWriter, r *http.Request) {
	videoID := r.URL.Query().Get("id")
	if videoID == "" {
		app.logger.Error("missing video id")
		app.badRequestError(w, r, errors.New("missing video id for SSE"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ctx := r.Context()
	
	// Channels to manage the asynchronous stream of messages and catch potential connection errors.
	msgCh := make(chan string)
	errCh := make(chan error)

	// Spin up a background worker to continuously listen for pub/sub events on this video's channel.
	// We do this concurrently so the primary handler thread remains unblocked to manage
	// the long-lived SSE HTTP connection cleanly.
	go func() {
		channelName := fmt.Sprintf("video:%s", videoID)
		err := app.queueMgr.ValkeyClient.Receive(ctx, app.queueMgr.ValkeyClient.B().Subscribe().Channel(channelName).Build(), func(msg valkey.PubSubMessage) {
			msgCh <- msg.Message
		})
		if err != nil {
			errCh <- err
		}
	}()

	flusher, _ := w.(http.Flusher)

	for {
		select {
		case <-ctx.Done():
			return
		case err := <-errCh:
			app.logger.Error("sse stream error", "err", err)
			return
		case msg := <-msgCh:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		}
	}
}
