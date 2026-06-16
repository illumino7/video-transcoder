import videojs from 'video.js';

if (typeof window !== 'undefined') {
  (window as any).videojs = videojs;
}
