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

type StatusMessage struct {
	UUID      string `json:"id"`
	Processed bool   `json:"processed"`
	Status    string `json:"status"`
}

// HandleTranscodeTask downloads the video from S3, transcodes it, uploads HLS output back to S3.
func (app *application) HandleTranscodeTask(ctx context.Context, payloadBytes []byte) error {
	var payload queue.TranscodePayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return err
	}

	app.logger.Info("started transcode pipeline", "video_id", payload.VideoID, "ext", payload.Ext)

	uploadsDir := env.GetString("UPLOADS_DIR", "./uploads")
	outputsDir := env.GetString("OUTPUTS_DIR", "./outputs")
	scriptsDir := env.GetString("SCRIPTS_DIR", "./scripts")

	// Resolve to absolute paths (Docker bind-mounts require absolute paths)
	uploadsDir, _ = filepath.Abs(uploadsDir)
	outputsDir, _ = filepath.Abs(outputsDir)
	scriptsDir, _ = filepath.Abs(scriptsDir)

	// Ensure directories exist
	os.MkdirAll(uploadsDir, 0o755)
	os.MkdirAll(outputsDir, 0o755)

	// Build the local file references from S3 key infered by video ID and ext
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

	// Run FFmpeg in a container — blocks until transcoding completes, auto-removes on exit
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

	// Upload transcoded HLS output to the streaming bucket
	outputDir := filepath.Join(outputsDir, payload.VideoID)
	s3Prefix := payload.VideoID

	if err := app.s3.UploadDir(ctx, StreamingBucket, s3Prefix, outputDir); err != nil {
		app.logger.Error("failed to upload HLS output to S3", "err", err)
		return fmt.Errorf("s3 upload failed: %w", err)
	}
	app.logger.Info("uploaded HLS output to streaming bucket", "prefix", s3Prefix)

	// Clean up local output directory
	if err := os.RemoveAll(outputDir); err != nil {
		app.logger.Error("failed to clean up local output", "dir", outputDir, "err", err)
	}

	// Publish completion event via Valkey
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
