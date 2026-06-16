package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/valkey-io/valkey-go"
)

type StatusMessage struct {
	UUID      string `json:"id"`
	Processed bool   `json:"processed"`
	Status    string `json:"status"`
}

// ssestatusHandler stream status updates for a video transcode request using SSE.
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
	msgCh := make(chan string, 1)
	errCh := make(chan error, 1)

	// Subscribe first to ensure no event is missed.
	go func() {
		channelName := fmt.Sprintf("video:%s", videoID)
		err := app.queueMgr.ValkeyClient.Receive(ctx, app.queueMgr.ValkeyClient.B().Subscribe().Channel(channelName).Build(), func(msg valkey.PubSubMessage) {
			msgCh <- msg.Message
		})
		if err != nil {
			errCh <- err
		}
	}()

	// Query DB after subscribing to catch fast transcodes.
	video, err := app.store.Videos.Get(ctx, videoID)
	if err == nil {
		if video.Status == "COMPLETED" || video.Status == "FAILED" {
			msg := StatusMessage{
				UUID:      videoID,
				Processed: true,
				Status:    video.Status,
			}
			data, _ := json.Marshal(msg)
			fmt.Fprintf(w, "data: %s\n\n", string(data))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			return
		}
	}

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
			return
		}
	}
}
