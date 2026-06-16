import { useRef, useEffect } from 'react';
import './videojs-globals'; // MUST be imported first to establish window.videojs before plugin modules execute
import videojs from 'video.js';
import type Player from 'video.js/dist/types/player';

import 'video.js/dist/video-js.css';
import 'jb-videojs-hls-quality-selector/dist/videojs-hls-quality-selector.css';

import 'videojs-contrib-quality-levels';
import 'jb-videojs-hls-quality-selector';

import { MINIO_URL } from '../config/constants';

// ── Types ──────────────────────────────────────────────
// Prop shapes defining the interface for the player component.

interface VideoPlayerProps {
  videoId: string;
}

// ── Component ──────────────────────────────────────────
// VideoPlayer serves as a React wrapper around the imperative video.js library.
// It explicitly constructs the HLS streaming engine and defensively tears it down 
// upon component unmount to prevent orphaned DOM nodes and memory leaks.

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
    if (typeof (player as any).qualityLevels === 'function') {
      (player as any).qualityLevels();
    } else {
      console.warn('qualityLevels plugin not registered on video.js player');
    }

    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    if (typeof (player as any).hlsQualitySelector === 'function') {
      (player as any).hlsQualitySelector({
        displayCurrentQuality: true,
      });
    } else {
      console.warn('hlsQualitySelector plugin not registered on video.js player');
    }

    player.src({
      src: `${MINIO_URL}/streaming/${videoId}/master.m3u8`,
      type: 'application/x-mpegURL',
    });

    // Cleanup: Destructively dispose the player instance when the React tree 
    // unmounts to free up video decoding threads and prevent phantom audio.
    return () => {
      if (playerRef.current) {
        playerRef.current.dispose();
        playerRef.current = null;
      }
    };
  }, [videoId]);

  return (
    <div className="w-full max-w-3xl mx-auto animate-fade-in-up">
      <div className="rounded-md overflow-hidden border border-zinc-800 bg-black" ref={containerRef} />
    </div>
  );
}
