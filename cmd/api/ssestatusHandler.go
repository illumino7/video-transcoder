package main

import (
	"errors"
	"fmt"
	"net/http"
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
	sub := app.rdb.Subscribe(ctx, fmt.Sprintf("video:%s", videoID))
	defer sub.Close()
	ch := sub.Channel()
	flusher, _ := w.(http.Flusher)

	for msg := range ch {
		fmt.Fprintf(w, "data: %s\n\n", msg.Payload)
		flusher.Flush()
	}

}
