package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/Eyevinn/hls-m3u8/m3u8"
	"github.com/theluminousartemis/video-transcoder/internal/env"
	"github.com/theluminousartemis/video-transcoder/internal/queue"
)

type StatusMessage struct {
	UUID      string `json:"id"`
	Processed bool   `json:"processed"`
	Status    string `json:"status"`
}

type AudioTrack struct {
	Index string
	Lang  string
}

// HandleTranscodeTask executes the HLS transcoding pipeline directly in Go.
func (app *application) HandleTranscodeTask(ctx context.Context, payloadBytes []byte) error {
	var payload queue.TranscodePayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return err
	}

	video, err := app.store.Videos.Get(ctx, payload.VideoID)
	if err != nil {
		return fmt.Errorf("fetch video status: %w", err)
	}

	if video.Status == "COMPLETED" {
		app.logger.Info("video transcode already completed, skipping", "video_id", payload.VideoID)
		return nil
	}

	if video.Status == "PROCESSING" {
		app.logger.Warn("video was mid-flight on a previous worker attempt, re-running transcode", "video_id", payload.VideoID)
	}

	if err := app.store.Videos.UpdateStatus(ctx, payload.VideoID, "PROCESSING"); err != nil {
		return fmt.Errorf("update status to PROCESSING: %w", err)
	}

	app.logger.Info("started transcode pipeline", "video_id", payload.VideoID, "ext", payload.Ext)

	uploadsDir := env.GetString("UPLOADS_DIR", "./uploads")
	outputsDir := env.GetString("OUTPUTS_DIR", "./outputs")

	uploadsDir, _ = filepath.Abs(uploadsDir)
	outputsDir, _ = filepath.Abs(outputsDir)

	os.MkdirAll(uploadsDir, 0o755)
	os.MkdirAll(outputsDir, 0o755)

	outputDir := filepath.Join(outputsDir, payload.VideoID)
	defer func() {
		if err := os.RemoveAll(outputDir); err != nil {
			app.logger.Error("clean up local output", "dir", outputDir, "err", err)
		}
	}()

	s3Key := fmt.Sprintf("%s%s", payload.VideoID, payload.Ext)
	filename := filepath.Base(s3Key)
	localInput := filepath.Join(uploadsDir, filename)

	app.logger.Info("downloading raw video from S3", "s3Key", s3Key, "dest", localInput)
	if err := app.s3.Download(ctx, UploadsBucket, s3Key, localInput); err != nil {
		_ = app.store.Videos.UpdateStatus(ctx, payload.VideoID, "FAILED")
		app.broadcastStatus(ctx, payload.VideoID, "FAILED")
		return fmt.Errorf("download video: %w", err)
	}
	defer func() {
		if err := os.Remove(localInput); err != nil {
			app.logger.Error("clean up local input", "file", localInput, "err", err)
		}
	}()

	probeCmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-select_streams", "a",
		"-show_entries", "stream=index:stream_tags=language", "-of", "csv=p=0", localInput)
	probeOut, err := probeCmd.Output()
	if err != nil {
		app.logger.Warn("ffprobe check for audio failed or no audio found", "err", err)
	}

	var audioTracks []AudioTrack
	for _, line := range strings.Split(string(probeOut), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) > 0 {
			idx := parts[0]
			lang := "und"
			if len(parts) > 1 && parts[1] != "" {
				lang = parts[1]
			}
			audioTracks = append(audioTracks, AudioTrack{Index: idx, Lang: lang})
		}
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(audioTracks)+3)

	for _, track := range audioTracks {
		wg.Add(1)
		go func(t AudioTrack) {
			defer wg.Done()
			audioDir := filepath.Join(outputDir, "audio", t.Lang)
			if err := os.MkdirAll(audioDir, 0o755); err != nil {
				errChan <- fmt.Errorf("create audio dir: %w", err)
				return
			}

			cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", localInput,
				"-map", fmt.Sprintf("0:a:%s?", t.Index), "-vn",
				"-c:a", "aac", "-profile:a", "aac_low", "-b:a", "128k",
				"-f", "hls", "-hls_time", "4", "-hls_playlist_type", "vod",
				"-hls_segment_type", "fmp4", "-hls_flags", "independent_segments",
				"-hls_fmp4_init_filename", "init.mp4",
				"-hls_segment_filename", filepath.Join(audioDir, "audio_%03d.m4s"),
				filepath.Join(audioDir, "audio.m3u8"),
			)
			if err := cmd.Run(); err != nil {
				errChan <- fmt.Errorf("extract audio (lang=%s): %w", t.Lang, err)
			}
		}(track)
	}

	variants := []struct {
		label string
		res   string
		br    string
	}{
		{"360p", "640x360", "500k"},
		{"480p", "854x480", "1250k"},
		{"720p", "1280x720", "2500k"},
	}

	for _, v := range variants {
		wg.Add(1)
		go func(label, res, br string) {
			defer wg.Done()
			segDir := filepath.Join(outputDir, label)
			if err := os.MkdirAll(segDir, 0o755); err != nil {
				errChan <- fmt.Errorf("create video dir %s: %w", label, err)
				return
			}

			cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", localInput,
				"-c:v", "libx265", "-tag:v", "hvc1", "-preset", "medium", "-b:v", br, "-s", res,
				"-pix_fmt", "yuv420p", "-an",
				"-f", "hls", "-hls_time", "4", "-hls_playlist_type", "vod",
				"-hls_segment_type", "fmp4", "-hls_flags", "independent_segments",
				"-hls_fmp4_init_filename", "init.mp4",
				"-hls_segment_filename", filepath.Join(segDir, "segment_%03d.m4s"),
				filepath.Join(segDir, "playlist.m3u8"),
			)
			if err := cmd.Run(); err != nil {
				errChan <- fmt.Errorf("video transcode (%s): %w", label, err)
			}
		}(v.label, v.res, v.br)
	}

	wg.Wait()
	close(errChan)

	var transcodeErrors []error
	for err := range errChan {
		transcodeErrors = append(transcodeErrors, err)
	}

	if len(transcodeErrors) > 0 {
		_ = app.store.Videos.UpdateStatus(ctx, payload.VideoID, "FAILED")
		app.broadcastStatus(ctx, payload.VideoID, "FAILED")
		return fmt.Errorf("transcode failures: %v", transcodeErrors)
	}

	masterPlay := m3u8.NewMasterPlaylist()
	masterPlay.SetVersion(7)

	var alternatives []*m3u8.Alternative
	for i, track := range audioTracks {
		isDefault := false
		if i == 0 {
			isDefault = true
		}
		alt := &m3u8.Alternative{
			Type:       "AUDIO",
			GroupId:    "audio-" + track.Lang,
			Name:       track.Lang,
			Default:    isDefault,
			Autoselect: true,
			URI:        fmt.Sprintf("audio/%s/audio.m3u8", track.Lang),
		}
		alternatives = append(alternatives, alt)
	}

	parseBandwidth := func(br string) uint32 {
		br = strings.TrimSuffix(br, "k")
		val, _ := strconv.Atoi(br)
		return uint32(val * 1000)
	}

	for _, v := range variants {
		bw := parseBandwidth(v.br)
		uri := fmt.Sprintf("%s/playlist.m3u8", v.label)

		if len(audioTracks) > 0 {
			seenLangs := make(map[string]bool)
			for _, track := range audioTracks {
				if seenLangs[track.Lang] {
					continue
				}
				seenLangs[track.Lang] = true

				params := m3u8.VariantParams{
					Bandwidth:    bw,
					Resolution:   v.res,
					Codecs:       "hvc1.1.6.L123,mp4a.40.2",
					Audio:        "audio-" + track.Lang,
					Alternatives: alternatives,
				}
				masterPlay.Append(uri, nil, params)
			}
		} else {
			params := m3u8.VariantParams{
				Bandwidth:  bw,
				Resolution: v.res,
				Codecs:     "hvc1.1.6.L123",
			}
			masterPlay.Append(uri, nil, params)
		}
	}

	masterPath := filepath.Join(outputDir, "master.m3u8")
	masterFile, err := os.Create(masterPath)
	if err != nil {
		_ = app.store.Videos.UpdateStatus(ctx, payload.VideoID, "FAILED")
		app.broadcastStatus(ctx, payload.VideoID, "FAILED")
		return fmt.Errorf("create master playlist: %w", err)
	}

	_, err = masterFile.Write(masterPlay.Encode().Bytes())
	masterFile.Close()
	if err != nil {
		_ = app.store.Videos.UpdateStatus(ctx, payload.VideoID, "FAILED")
		app.broadcastStatus(ctx, payload.VideoID, "FAILED")
		return fmt.Errorf("write master playlist: %w", err)
	}

	s3Prefix := payload.VideoID
	if err := app.s3.UploadDir(ctx, StreamingBucket, s3Prefix, outputDir); err != nil {
		app.logger.Error("failed to upload HLS output to S3", "err", err)
		_ = app.store.Videos.UpdateStatus(ctx, payload.VideoID, "FAILED")
		app.broadcastStatus(ctx, payload.VideoID, "FAILED")
		return fmt.Errorf("s3 upload HLS: %w", err)
	}
	app.logger.Info("uploaded HLS output to streaming bucket", "prefix", s3Prefix)

	if err := app.s3.Delete(ctx, UploadsBucket, s3Key); err != nil {
		app.logger.Error("failed to delete raw upload from S3", "key", s3Key, "err", err)
	} else {
		app.logger.Info("deleted raw upload from S3", "key", s3Key)
	}

	if err := app.store.Videos.UpdateStatus(ctx, payload.VideoID, "COMPLETED"); err != nil {
		app.logger.Error("failed to update status to COMPLETED", "err", err)
	}

	app.broadcastStatus(ctx, payload.VideoID, "COMPLETED")
	return nil
}

func (app *application) broadcastStatus(ctx context.Context, videoID, status string) {
	statusMessage := StatusMessage{
		UUID:      videoID,
		Processed: true,
		Status:    status,
	}

	data, _ := json.Marshal(statusMessage)
	channelName := fmt.Sprintf("video:%s", videoID)
	if err := app.queueMgr.ValkeyClient.Do(ctx, app.queueMgr.ValkeyClient.B().Publish().Channel(channelName).Message(string(data)).Build()).Error(); err != nil {
		app.logger.Error("failed to publish status message", "channel", channelName, "err", err)
	}
}
