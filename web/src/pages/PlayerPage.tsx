import { useNavigate, useParams } from 'react-router-dom';
import Header from '../components/Header';
import VideoPlayer from '../components/VideoPlayer';

export default function PlayerPage() {
  const { videoId } = useParams<{ videoId: string }>();
  const navigate = useNavigate();

  if (!videoId) return null;

  return (
    <div className="flex flex-col items-center min-h-dvh p-8 max-sm:p-6">
      <Header showSubtitle={false} />
      <div className="w-full max-w-[960px] animate-fade-in-up">
        <VideoPlayer videoId={videoId} />
        <div className="flex justify-center mt-6">
          <button
            className="
              inline-flex items-center gap-2 py-2.5 px-5
              bg-white/[0.04] border border-white/[0.08] rounded-xl
              text-zinc-400 text-sm font-medium cursor-pointer
              transition-all duration-250
              hover:bg-white/[0.07] hover:text-zinc-200 hover:border-indigo-500/40
            "
            onClick={() => navigate('/')}
          >
            ← Upload another video
          </button>
        </div>
      </div>
    </div>
  );
}
