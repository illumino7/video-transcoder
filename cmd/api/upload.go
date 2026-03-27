package main

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/theluminousartemis/video-transcoder/internal/queue"
	"github.com/theluminousartemis/video-transcoder/internal/storage"
)

// presignUpload generates a presigned PUT URL so the client can upload directly to MinIO.
// GET /api/v1/upload/presign?filename=video.mkv
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

	presignedURL, err := app.s3.PresignedPutURL(r.Context(), UploadsBucket, objectKey, 15*time.Minute)
	if err != nil {
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

// uploadComplete is called after the client finishes uploading to MinIO.
// It verifies the file exists in S3, validates it, and enqueues the transcode job.
// POST /api/v1/upload  body: { "videoId": "uuid", "s3Key": "uuid.mkv" }
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

	// Verify the object actually exists in the uploads bucket
	info, err := app.s3.StatObject(r.Context(), UploadsBucket, body.S3Key)
	if err != nil {
		app.badRequestError(w, r, fmt.Errorf("file not found in storage: %w", err))
		return
	}

	// Validate file size (100 MB limit)
	const maxSize = 100 * 1024 * 1024
	if info.Size > maxSize {
		app.badRequestError(w, r, fmt.Errorf("file too large: %d bytes (max %d)", info.Size, maxSize))
		return
	}

	// Validate content type
	ct := storage.DetectContentType(body.S3Key)
	if ct == "application/octet-stream" {
		app.badRequestError(w, r, errors.New("unsupported file format"))
		return
	}

	// Add transcode job to Valkey stream
	ext := filepath.Ext(body.S3Key)
	err = queue.EnqueueTranscode(r.Context(), app.queueMgr.ValkeyClient, body.VideoID, ext)
	if err != nil {
		app.logger.Error("failed to enqueue transcode job", "err", err, "video_id", body.VideoID)
		http.Error(w, `{"error":"failed to queue job"}`, http.StatusInternalServerError)
		return
	}

	WriteJSON(w, http.StatusOK, map[string]string{
		"message": "transcode job enqueued",
		"videoId": body.VideoID,
	})
}
