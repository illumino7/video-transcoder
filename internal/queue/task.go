package queue

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/valkey-io/valkey-go"
)

// TranscodePayload represents the standardized data contract required to execute
// a video transcode. It is serialized to JSON and attached to the stream message.
type TranscodePayload struct {
	VideoID string `json:"video_id"`
	Ext     string `json:"ext"` // e.g. .mp4
}

// EnqueueTranscode pushes a new transcode job into the primary Valkey stream.
// It acts as the boundary handoff between the HTTP API accepting an upload and
// the async worker pool picking up the actual FFmpeg processing compute.
func EnqueueTranscode(ctx context.Context, client valkey.Client, videoID string, ext string) error {
	slog.Info("enqueue transcode", "videoID", videoID, "ext", ext)
	payload := TranscodePayload{
		VideoID: videoID,
		Ext:     ext,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	cmd := client.B().Xadd().Key(StreamTranscode).Id("*").
		FieldValue().FieldValue("payload", string(payloadBytes)).Build()

	return client.Do(ctx, cmd).Error()
}
