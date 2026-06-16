import { useNavigate, useParams } from 'react-router-dom';
import Header from '../components/Header';
import VideoPlayer from '../components/VideoPlayer';

// PlayerPage handles the playback route (/video/:videoId). 
// It safely extracts the video ID from the URL parameters and mounts the 
// isolated VideoPlayer component, ensuring playback state doesn't leak.
export default function PlayerPage() {
  const { videoId } = useParams<{ videoId: string }>();
  const navigate = useNavigate();

  if (!videoId) return null;

  return (
    <div className="flex flex-col items-center justify-center min-h-dvh p-8 max-sm:p-6 bg-black text-white selection:bg-zinc-800">
      <div className="w-full max-w-3xl animate-fade-in-up">
        <Header showSubtitle={false} />
        <VideoPlayer videoId={videoId} />
        <div className="flex justify-center mt-8">
          <button
            className="
              inline-flex items-center gap-2 py-2 px-4
              bg-transparent border border-zinc-800 rounded-md
              text-zinc-400 text-xs font-medium cursor-pointer
              transition-colors duration-150
              hover:bg-zinc-900 hover:text-zinc-200 hover:border-zinc-700
              focus-visible:ring-1 focus-visible:ring-zinc-400 focus-visible:outline-none
            "
            onClick={() => navigate('/')}
            aria-label="Navigate back to upload another video page"
          >
            &larr; Upload another video
          </button>
        </div>
      </div>
    </div>
  );
}
