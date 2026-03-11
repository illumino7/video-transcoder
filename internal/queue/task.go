package queue

import (
	"encoding/json"
	"log/slog"

	"github.com/hibiken/asynq"
)

type TranscodePayload struct {
	VideoID string `json:"video_id"`
	S3Key   string `json:"s3_key"`
}

func EnqueueTranscode(client *asynq.Client, videoID, s3Key string) error {
	slog.Info("enqueue transcode", "videoID", videoID, "s3Key", s3Key)
	payload, err := json.Marshal(TranscodePayload{
		VideoID: videoID,
		S3Key:   s3Key,
	})
	if err != nil {
		return err
	}

	info, err := client.Enqueue(asynq.NewTask(TypeVideoTranscode, payload))
	jsonInfo, _ := json.Marshal(info)
	slog.Info("task information", "info", string(jsonInfo))
	return err
}
