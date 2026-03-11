#!/usr/bin/env bash
set -euo pipefail

INPUT="$1"
UUID=$(basename "$INPUT" | cut -d. -f1)
WORKDIR="/outputs/$UUID"
mkdir -p "$WORKDIR"

RESOLUTIONS=("640x360" "854x480" "1280x720")
BITRATES=("500k" "1250k" "2500k")
LABELS=("360p" "480p" "720p")

echo "Processing: $INPUT"
echo "Output: $WORKDIR"

# ──────────────────────────────────────────────────────────────
# PHASE 1 — Extract audio tracks (sequential)
# ──────────────────────────────────────────────────────────────
AUDIO_TRACKS=$(ffprobe -v error -select_streams a \
  -show_entries stream=index:stream_tags=language \
  -of csv=p=0 "$INPUT" || true)

if [[ -z "$AUDIO_TRACKS" ]]; then
  echo "No audio tracks detected, proceeding video-only!"
else
  echo "Detected audio tracks:"
  echo "$AUDIO_TRACKS"
fi

AUDIO_ENTRIES=""
INDEX_COUNT=0
while IFS=, read -r INDEX LANG; do
  [[ -z "$INDEX" ]] && continue
  LANG=${LANG:-und}
  AUDIO_DIR="$WORKDIR/audio/$LANG"
  mkdir -p "$AUDIO_DIR"

  echo "Extracting audio track index $INDEX (lang=$LANG)..."
  ffmpeg -y -i "$INPUT" \
    -map 0:a:${INDEX}? -c:a aac -b:a 128k \
    -hls_time 4 -hls_playlist_type vod \
    -hls_segment_filename "$AUDIO_DIR/audio_%03d.ts" \
    "$AUDIO_DIR/audio.m3u8"

  if [[ $INDEX_COUNT -eq 0 ]]; then
    AUDIO_ENTRIES+="#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID=\"audio-$LANG\",NAME=\"$LANG\",DEFAULT=YES,AUTOSELECT=YES,URI=\"audio/$LANG/audio.m3u8\""$'\n'
  else
    AUDIO_ENTRIES+="#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID=\"audio-$LANG\",NAME=\"$LANG\",DEFAULT=NO,AUTOSELECT=YES,URI=\"audio/$LANG/audio.m3u8\""$'\n'
  fi
  INDEX_COUNT=$((INDEX_COUNT+1))
done <<< "$AUDIO_TRACKS"

# ──────────────────────────────────────────────────────────────
# PHASE 2 — Encode video renditions in PARALLEL
# ──────────────────────────────────────────────────────────────
PIDS=()
for i in "${!RESOLUTIONS[@]}"; do
  RES="${RESOLUTIONS[$i]}"
  BR="${BITRATES[$i]}"
  LABEL="${LABELS[$i]}"
  SEG_DIR="$WORKDIR/$LABEL"
  mkdir -p "$SEG_DIR"

  echo "Encoding video $LABEL ($RES @ $BR) [parallel]..."

  ffmpeg -y -i "$INPUT" \
    -c:v libx264 -tag:v avc1 -preset medium -b:v "$BR" -s "$RES" \
    -pix_fmt yuv420p -an \
    -hls_time 4 -hls_playlist_type vod \
    -hls_segment_filename "$SEG_DIR/segment_%03d.ts" \
    "$SEG_DIR/playlist.m3u8" &

  PIDS+=($!)
done

# Wait for all parallel renditions to finish
FAIL=0
for pid in "${PIDS[@]}"; do
  if ! wait "$pid"; then
    echo "ERROR: rendition PID $pid failed"
    FAIL=1
  fi
done

if [[ $FAIL -ne 0 ]]; then
  echo "One or more renditions failed!"
  exit 1
fi

# ──────────────────────────────────────────────────────────────
# PHASE 3 — Assemble master.m3u8 (no race conditions)
# ──────────────────────────────────────────────────────────────
MASTER="$WORKDIR/master.m3u8"
{
  echo "#EXTM3U"
  echo "#EXT-X-VERSION:3"
  printf "%s" "$AUDIO_ENTRIES"

  for i in "${!RESOLUTIONS[@]}"; do
    RES="${RESOLUTIONS[$i]}"
    BR="${BITRATES[$i]}"
    LABEL="${LABELS[$i]}"
    BW=$(( ${BR%k} * 1000 ))

    if [[ -n "$AUDIO_TRACKS" ]]; then
      for LANG in $(echo "$AUDIO_TRACKS" | cut -d, -f2 | sort -u); do
        LANG=${LANG:-und}
        echo "#EXT-X-STREAM-INF:BANDWIDTH=$BW,RESOLUTION=$RES,CODECS=\"avc1.4d401f,mp4a.40.2\",AUDIO=\"audio-$LANG\""
        echo "$LABEL/playlist.m3u8"
      done
    else
      echo "#EXT-X-STREAM-INF:BANDWIDTH=$BW,RESOLUTION=$RES,CODECS=\"avc1.4d401f\""
      echo "$LABEL/playlist.m3u8"
    fi
  done
} > "$MASTER"

echo "HLS Transcoding Complete!"
echo "Master Playlist: $MASTER"
