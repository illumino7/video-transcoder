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
	
	// Create channels for streaming
	msgCh := make(chan string)
	errCh := make(chan error)

	// In a goroutine, listen to Valkey pub/sub
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
