package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/theluminousartemis/video-transcoder/internal/queue"
	"github.com/theluminousartemis/video-transcoder/internal/storage"
)

// presignUpload generates a presigned URL allowing direct raw uploads to S3/MinIO.
func (app *application) presignUpload(w http.ResponseWriter, r *http.Request) {
	filename := r.URL.Query().Get("filename")
	if filename == "" {
		app.badRequestError(w, r, errors.New("filename query param is required"))
		return
	}

	id := uuid.New().String()
	ext := filepath.Ext(filename)
	objectKey := fmt.Sprintf("%s%s", id, ext)
	contentType := storage.DetectContentType(filename)

	presignedURL, err := app.s3.PresignedPutURL(r.Context(), UploadsBucket, objectKey, 15*time.Minute, contentType)
	if err != nil {
		app.logger.Error("failed to generate presigned upload URL", "video_id", id, "s3_key", objectKey, "err", err)
		app.internalServerError(w, r, err)
		return
	}

	WriteJSON(w, http.StatusOK, map[string]string{
		"videoId":     id,
		"uploadUrl":   presignedURL.String(),
		"s3Key":       objectKey,
		"contentType": contentType,
	})
}

// uploadComplete handles client upload confirmations and enqueues transcode jobs.
func (app *application) uploadComplete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		VideoID string `json:"videoId"`
		S3Key   string `json:"s3Key"`
	}
	if err := ReadJSON(w, r, &body); err != nil {
		app.badRequestError(w, r, err)
		return
	}
	if body.VideoID == "" || body.S3Key == "" {
		app.badRequestError(w, r, errors.New("videoId and s3Key are required"))
		return
	}

	info, err := app.s3.StatObject(r.Context(), UploadsBucket, body.S3Key)
	if err != nil {
		app.logger.Warn("failed to verify upload file existence in storage", "s3_key", body.S3Key, "err", err)
		app.badRequestError(w, r, fmt.Errorf("file not found in storage: %w", err))
		return
	}

	const maxSize = 100 * 1024 * 1024
	if info.Size > maxSize {
		app.badRequestError(w, r, fmt.Errorf("file too large: %d bytes (max %d)", info.Size, maxSize))
		return
	}

	ct := storage.DetectContentType(body.S3Key)
	if ct == "application/octet-stream" {
		app.badRequestError(w, r, errors.New("unsupported file format"))
		return
	}

	ext := filepath.Ext(body.S3Key)
	payload := queue.TranscodePayload{
		VideoID: body.VideoID,
		Ext:     ext,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		app.logger.Error("failed to marshal transcode payload", "err", err, "video_id", body.VideoID)
		app.internalServerError(w, r, err)
		return
	}

	err = app.store.Videos.CreateWithOutbox(r.Context(), body.VideoID, ext, string(payloadBytes))
	if err != nil {
		app.logger.Error("failed to save transcode job to outbox", "err", err, "video_id", body.VideoID)
		app.internalServerError(w, r, err)
		return
	}

	WriteJSON(w, http.StatusOK, map[string]string{
		"message": "transcode job enqueued",
		"videoId": body.VideoID,
	})
}
