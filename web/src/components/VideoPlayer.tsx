import { useRef, useEffect } from 'react';
import videojs from 'video.js';
import type Player from 'video.js/dist/types/player';

import 'video.js/dist/video-js.css';
import 'jb-videojs-hls-quality-selector/dist/videojs-hls-quality-selector.css';

import 'videojs-contrib-quality-levels';
import 'jb-videojs-hls-quality-selector';

interface VideoPlayerProps {
  videoId: string;
}

const BACKEND_URL = 'http://localhost:3030';

export default function VideoPlayer({ videoId }: VideoPlayerProps) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const playerRef = useRef<Player | null>(null);

  useEffect(() => {
    if (!containerRef.current) return;

    // Create a fresh video element each time
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

    // Initialize quality levels plugin BEFORE setting the source
    // so it can intercept HLS manifest parsing events
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (player as any).qualityLevels();

    // Initialize quality selector plugin — adds the UI dropdown
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (player as any).hlsQualitySelector({
      displayCurrentQuality: true,
    });

    // Set source AFTER plugins are initialized
    player.src({
      src: `${BACKEND_URL}/videos/${videoId}/master.m3u8`,
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
    <div className="player-container">
      <div className="player-shell" ref={containerRef} />
    </div>
  );
}
