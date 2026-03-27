import { useRef, useEffect } from 'react';
import videojs from 'video.js';
import type Player from 'video.js/dist/types/player';

import 'video.js/dist/video-js.css';
import 'jb-videojs-hls-quality-selector/dist/videojs-hls-quality-selector.css';

import 'videojs-contrib-quality-levels';
import 'jb-videojs-hls-quality-selector';

import { MINIO_URL } from '../config/constants';

// ── Types ──────────────────────────────────────────────

interface VideoPlayerProps {
  videoId: string;
}

// ── Component ──────────────────────────────────────────

export default function VideoPlayer({ videoId }: VideoPlayerProps) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const playerRef = useRef<Player | null>(null);

  useEffect(() => {
    if (!containerRef.current) return;

    const videoEl = document.createElement('video');
    videoEl.classList.add('video-js', 'vjs-default-skin');
    videoEl.setAttribute('playsinline', '');
    containerRef.current.appendChild(videoEl);

    const player = videojs(videoEl, {
      controls: true,
      autoplay: false,
      preload: 'auto',
      fluid: true,
      aspectRatio: '16:9',
    }) as Player;

    playerRef.current = player;

    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (player as any).qualityLevels();

    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (player as any).hlsQualitySelector({
      displayCurrentQuality: true,
    });

    player.src({
      src: `${MINIO_URL}/streaming/${videoId}/master.m3u8`,
      type: 'application/x-mpegURL',
    });

    return () => {
      if (playerRef.current) {
        playerRef.current.dispose();
        playerRef.current = null;
      }
    };
  }, [videoId]);

  return (
    <div className="w-full max-w-[960px] animate-fade-in-up">
      <div className="rounded-2xl overflow-hidden border border-white/[0.08] shadow-[0_4px_40px_rgba(0,0,0,0.4)]" ref={containerRef} />
    </div>
  );
}
