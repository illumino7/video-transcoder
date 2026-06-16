package queue

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/valkey-io/valkey-go"
)

type TranscodePayload struct {
	VideoID string `json:"video_id"`
	Ext     string `json:"ext"`
}

// EnqueueTranscode pushes a new transcode job into the primary Valkey stream.
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
