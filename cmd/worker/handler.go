package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/theluminousartemis/video-transcoder/internal/env"
	"github.com/theluminousartemis/video-transcoder/internal/queue"
)

// StatusMessage represents the payload dispatched to Valkey upon successful transcode.
// It is used by the API nodes to forward Server-Sent Events (SSE) to connected clients.
type StatusMessage struct {
	UUID      string `json:"id"`
	Processed bool   `json:"processed"`
	Status    string `json:"status"`
}

// HandleTranscodeTask is the primary worker function executing the transcode pipeline.
// It orchestrates downloading raw video from S3, running FFmpeg, and uploading the
// resulting HLS playlist and segments back to the streaming bucket.
func (app *application) HandleTranscodeTask(ctx context.Context, payloadBytes []byte) error {
	var payload queue.TranscodePayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return err
	}

	app.logger.Info("started transcode pipeline", "video_id", payload.VideoID, "ext", payload.Ext)

	uploadsDir := env.GetString("UPLOADS_DIR", "./uploads")
	outputsDir := env.GetString("OUTPUTS_DIR", "./outputs")
	scriptsDir := env.GetString("SCRIPTS_DIR", "./scripts")

	// Resolve to absolute paths since Docker bind-mounts (-v) strictly require 
	// absolute paths to properly map host directories to the container filesystem.
	uploadsDir, _ = filepath.Abs(uploadsDir)
	outputsDir, _ = filepath.Abs(outputsDir)
	scriptsDir, _ = filepath.Abs(scriptsDir)

	// Ensure destination directories exist prior to downloading files or 
	// spinning up the container, otherwise Docker will generate them with root permissions.
	os.MkdirAll(uploadsDir, 0o755)
	os.MkdirAll(outputsDir, 0o755)

	// Construct the S3 key structure based on inference from the payload UUID and extension.
	s3Key := fmt.Sprintf("%s%s", payload.VideoID, payload.Ext)
	filename := filepath.Base(s3Key)
	localInput := filepath.Join(uploadsDir, filename)

	app.logger.Info("downloading raw video from S3", "s3Key", s3Key, "dest", localInput)
	if err := app.s3.Download(ctx, UploadsBucket, s3Key, localInput); err != nil {
		return fmt.Errorf("failed to download video from S3: %w", err)
	}
	defer func() {
		if err := os.Remove(localInput); err != nil {
			app.logger.Error("failed to clean up local input", "file", localInput, "err", err)
		}
	}()

	// We run FFmpeg in an isolated Docker container rather than installing it directly on
	// the host system to ensure a static, reproducible transcode environment. The execution
	// safely blocks until the process completely exits and the --rm flag ensures no disk bloat.
	uid := os.Getuid()
	gid := os.Getgid()

	cmd := exec.Command(
		"docker", "run", "--rm",
		"--user", fmt.Sprintf("%d:%d", uid, gid),
		"-v", fmt.Sprintf("%s:/uploads:ro", uploadsDir),
		"-v", fmt.Sprintf("%s:/outputs", outputsDir),
		"-v", fmt.Sprintf("%s:/scripts:ro", scriptsDir),
		"--entrypoint", "bash",
		"jrottenberg/ffmpeg:latest",
		"/scripts/transcodeh265.sh", fmt.Sprintf("/uploads/%s", filename),
	)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	app.logger.Info("docker command", "args", cmd.Args)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg transcode failed: %w", err)
	}

	// Upload the fully transcoded HLS stream (the .m3u8 playlist and associated .ts segments)
	// directly into the S3 bucket designated for streaming output.
	outputDir := filepath.Join(outputsDir, payload.VideoID)
	s3Prefix := payload.VideoID

	if err := app.s3.UploadDir(ctx, StreamingBucket, s3Prefix, outputDir); err != nil {
		app.logger.Error("failed to upload HLS output to S3", "err", err)
		return fmt.Errorf("s3 upload failed: %w", err)
	}
	app.logger.Info("uploaded HLS output to streaming bucket", "prefix", s3Prefix)

	// Clean up local processing artifacts defensively to avoid filling up the worker node's
	// persistent storage across multiple transcode executions.
	if err := os.RemoveAll(outputDir); err != nil {
		app.logger.Error("failed to clean up local output", "dir", outputDir, "err", err)
	}

	// Broadcast a message to the pub/sub channel notifying API nodes and clients
	// that this specific video is successfully processed and ready for streaming.
	statusMessage := StatusMessage{
		UUID:      payload.VideoID,
		Processed: true,
		Status:    "Completed",
	}

	data, _ := json.Marshal(statusMessage)

	if err := app.queueMgr.ValkeyClient.Do(ctx, app.queueMgr.ValkeyClient.B().Publish().Channel(fmt.Sprintf("video:%s", payload.VideoID)).Message(string(data)).Build()).Error(); err != nil {
		app.logger.Error("failed to publish message", "err", err)
	}

	return nil
}
