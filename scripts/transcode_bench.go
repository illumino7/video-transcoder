package main

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

type Variant struct {
	label string
	res   string
	br    string
}

func main() {
	workDir := "./bench_work"
	err := os.MkdirAll(workDir, 0755)
	if err != nil {
		fmt.Printf("Error creating work dir: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		fmt.Println("Cleaning up temporary work directory...")
		os.RemoveAll(workDir)
	}()

	inputPath := filepath.Join(workDir, "input_sample.mp4")

	fmt.Println("Step 1: Generating a 5-second 1080p complex test video using FFmpeg...")
	// We use a combination of cellauto (cellular automaton) and noise to simulate realistic high-complexity video scenes
	genCmd := exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", "cellauto=size=1280x720:rate=30",
		"-f", "lavfi", "-i", "sine=frequency=1000",
		"-t", "5",
		"-pix_fmt", "yuv420p",
		"-c:v", "libx264", "-crf", "23",
		"-c:a", "aac", "-b:a", "128k",
		inputPath,
	)
	if out, err := genCmd.CombinedOutput(); err != nil {
		fmt.Printf("Error generating test video: %v\nOutput: %s\n", err, out)
		os.Exit(1)
	}

	inputStat, err := os.Stat(inputPath)
	if err != nil {
		fmt.Printf("Error stating input: %v\n", err)
		os.Exit(1)
	}
	inputSize := inputStat.Size()
	fmt.Printf("Input video generated. Size: %.2f MB\n", float64(inputSize)/(1024*1024))

	variants := []Variant{
		{"360p", "640x360", "500k"},
		{"480p", "854x480", "1250k"},
		{"720p", "1280x720", "2500k"},
	}

	// ==========================================
	// RUN 1: Fixed Bitrate (Production Mode)
	// ==========================================
	fmt.Println("\n==============================================")
	fmt.Println("RUN 1: Production Mode (Fixed Target Bitrates)")
	fmt.Println("==============================================")

	h265DirBitrate := filepath.Join(workDir, "h265_bitrate")
	h265Start := time.Now()
	if err := runTranscodeBitrate(inputPath, h265DirBitrate, "libx265", true, variants); err != nil {
		fmt.Printf("H.265 transcode failed: %v\n", err)
		os.Exit(1)
	}
	h265Dur := time.Since(h265Start)
	h265SizeBitrate := dirSize(h265DirBitrate)

	h264DirBitrate := filepath.Join(workDir, "h264_bitrate")
	h264Start := time.Now()
	if err := runTranscodeBitrate(inputPath, h264DirBitrate, "libx264", false, variants); err != nil {
		fmt.Printf("H.264 transcode failed: %v\n", err)
		os.Exit(1)
	}
	h264Dur := time.Since(h264Start)
	h264SizeBitrate := dirSize(h264DirBitrate)

	fmt.Printf("H.265/HEVC Out (Bitrate): %.2f MB (%v)\n", float64(h265SizeBitrate)/(1024*1024), h265Dur)
	fmt.Printf("H.264/AVC Out (Bitrate):  %.2f MB (%v)\n", float64(h264SizeBitrate)/(1024*1024), h264Dur)

	// ==========================================
	// RUN 2: Equivalent Quality Mode (CRF Mode)
	// ==========================================
	fmt.Println("\n==============================================")
	fmt.Println("RUN 2: Equivalent Visual Quality (CRF Mode)")
	fmt.Println("H.264 @ CRF 23 vs H.265 @ CRF 28")
	fmt.Println("==============================================")

	h265DirCRF := filepath.Join(workDir, "h265_crf")
	h265CRFStart := time.Now()
	if err := runTranscodeCRF(inputPath, h265DirCRF, "libx265", true, 28, variants); err != nil {
		fmt.Printf("H.265 CRF transcode failed: %v\n", err)
		os.Exit(1)
	}
	h265CRFDur := time.Since(h265CRFStart)
	h265SizeCRF := dirSize(h265DirCRF)

	h264DirCRF := filepath.Join(workDir, "h264_crf")
	h264CRFStart := time.Now()
	if err := runTranscodeCRF(inputPath, h264DirCRF, "libx264", false, 23, variants); err != nil {
		fmt.Printf("H.264 CRF transcode failed: %v\n", err)
		os.Exit(1)
	}
	h264CRFDur := time.Since(h264CRFStart)
	h264SizeCRF := dirSize(h264DirCRF)

	fmt.Printf("H.265/HEVC Out (CRF 28): %.2f MB (%v)\n", float64(h265SizeCRF)/(1024*1024), h265CRFDur)
	fmt.Printf("H.264/AVC Out (CRF 23):  %.2f MB (%v)\n", float64(h264SizeCRF)/(1024*1024), h264CRFDur)

	fmt.Println("\n==================================================")
	fmt.Println("              TRANSCODING METRICS REPORT          ")
	fmt.Println("==================================================")
	fmt.Printf("Input Raw Video Size:               %.2f MB\n", float64(inputSize)/(1024*1024))
	fmt.Println("\n--- Run 1: Fixed Bitrate (Production Config) ---")
	fmt.Printf("H.264 AVC HLS Output:               %.2f MB\n", float64(h264SizeBitrate)/(1024*1024))
	fmt.Printf("H.265 HEVC HLS Output:              %.2f MB\n", float64(h265SizeBitrate)/(1024*1024))
	diffBitrate := h264SizeBitrate - h265SizeBitrate
	pctBitrate := (float64(diffBitrate) / float64(h264SizeBitrate)) * 100
	fmt.Printf("Size Change:                        %.2f MB (%.2f%% change)\n", float64(diffBitrate)/(1024*1024), pctBitrate)

	fmt.Println("\n--- Run 2: Equivalent Quality (CRF 23 vs 28) ---")
	fmt.Printf("H.264 AVC (CRF 23) Output:          %.2f MB\n", float64(h264SizeCRF)/(1024*1024))
	fmt.Printf("H.265 HEVC (CRF 28) Output:         %.2f MB\n", float64(h265SizeCRF)/(1024*1024))
	diffCRF := h264SizeCRF - h265SizeCRF
	pctCRF := (float64(diffCRF) / float64(h264SizeCRF)) * 100
	fmt.Printf("True Coding Storage Savings:        %.2f MB (%.2f%% reduction)\n", float64(diffCRF)/(1024*1024), pctCRF)
	fmt.Println("==================================================")
}

func runTranscodeBitrate(inputPath, outDir string, codec string, isH265 bool, variants []Variant) error {
	os.MkdirAll(outDir, 0755)

	var wg sync.WaitGroup
	errChan := make(chan error, len(variants)+1)

	// Audio extract
	wg.Add(1)
	go func() {
		defer wg.Done()
		audioDir := filepath.Join(outDir, "audio", "und")
		os.MkdirAll(audioDir, 0755)

		cmd := exec.Command("ffmpeg", "-y", "-i", inputPath,
			"-map", "0:a:0?", "-vn",
			"-c:a", "aac", "-profile:a", "aac_low", "-b:a", "128k",
			"-f", "hls", "-hls_time", "4", "-hls_playlist_type", "vod",
			"-hls_segment_type", "fmp4", "-hls_flags", "independent_segments",
			"-hls_fmp4_init_filename", "init.mp4",
			"-hls_segment_filename", filepath.Join(audioDir, "audio_%03d.m4s"),
			filepath.Join(audioDir, "audio.m3u8"),
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			errChan <- fmt.Errorf("audio extract failed: %v, output: %s", err, out)
		}
	}()

	for _, v := range variants {
		wg.Add(1)
		go func(v Variant) {
			defer wg.Done()
			segDir := filepath.Join(outDir, v.label)
			os.MkdirAll(segDir, 0755)

			args := []string{
				"-y", "-i", inputPath,
				"-c:v", codec,
			}
			if isH265 {
				args = append(args, "-tag:v", "hvc1")
			}
			args = append(args,
				"-preset", "medium", "-b:v", v.br, "-s", v.res,
				"-pix_fmt", "yuv420p", "-an",
				"-f", "hls", "-hls_time", "4", "-hls_playlist_type", "vod",
				"-hls_segment_type", "fmp4", "-hls_flags", "independent_segments",
				"-hls_fmp4_init_filename", "init.mp4",
				"-hls_segment_filename", filepath.Join(segDir, "segment_%03d.m4s"),
				filepath.Join(segDir, "playlist.m3u8"),
			)

			cmd := exec.Command("ffmpeg", args...)
			if out, err := cmd.CombinedOutput(); err != nil {
				errChan <- fmt.Errorf("variant %s failed: %v, output: %s", v.label, err, out)
			}
		}(v)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		if err != nil {
			return err
		}
	}
	return nil
}

func runTranscodeCRF(inputPath, outDir string, codec string, isH265 bool, crf int, variants []Variant) error {
	os.MkdirAll(outDir, 0755)

	var wg sync.WaitGroup
	errChan := make(chan error, len(variants)+1)

	// Audio extract
	wg.Add(1)
	go func() {
		defer wg.Done()
		audioDir := filepath.Join(outDir, "audio", "und")
		os.MkdirAll(audioDir, 0755)

		cmd := exec.Command("ffmpeg", "-y", "-i", inputPath,
			"-map", "0:a:0?", "-vn",
			"-c:a", "aac", "-profile:a", "aac_low", "-b:a", "128k",
			"-f", "hls", "-hls_time", "4", "-hls_playlist_type", "vod",
			"-hls_segment_type", "fmp4", "-hls_flags", "independent_segments",
			"-hls_fmp4_init_filename", "init.mp4",
			"-hls_segment_filename", filepath.Join(audioDir, "audio_%03d.m4s"),
			filepath.Join(audioDir, "audio.m3u8"),
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			errChan <- fmt.Errorf("audio extract failed: %v, output: %s", err, out)
		}
	}()

	for _, v := range variants {
		wg.Add(1)
		go func(v Variant) {
			defer wg.Done()
			segDir := filepath.Join(outDir, v.label)
			os.MkdirAll(segDir, 0755)

			args := []string{
				"-y", "-i", inputPath,
				"-c:v", codec,
				"-crf", fmt.Sprintf("%d", crf),
			}
			if isH265 {
				args = append(args, "-tag:v", "hvc1")
			}
			args = append(args,
				"-preset", "medium", "-s", v.res,
				"-pix_fmt", "yuv420p", "-an",
				"-f", "hls", "-hls_time", "4", "-hls_playlist_type", "vod",
				"-hls_segment_type", "fmp4", "-hls_flags", "independent_segments",
				"-hls_fmp4_init_filename", "init.mp4",
				"-hls_segment_filename", filepath.Join(segDir, "segment_%03d.m4s"),
				filepath.Join(segDir, "playlist.m3u8"),
			)

			cmd := exec.Command("ffmpeg", args...)
			if out, err := cmd.CombinedOutput(); err != nil {
				errChan <- fmt.Errorf("variant %s failed: %v, output: %s", v.label, err, out)
			}
		}(v)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		if err != nil {
			return err
		}
	}
	return nil
}

func dirSize(path string) int64 {
	var size int64
	_ = filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			info, err := d.Info()
			if err == nil {
				size += info.Size()
			}
		}
		return nil
	})
	return size
}
