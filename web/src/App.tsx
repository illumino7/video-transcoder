import { BrowserRouter, Routes, Route, useNavigate, useParams } from 'react-router-dom';
import Upload from './components/Upload.tsx';
import VideoPlayer from './components/VideoPlayer.tsx';

function Header() {
  return (
    <header className="app-header">
      <h1>Video Transcoder</h1>
      <p>Upload a video and stream it in adaptive HLS quality</p>
    </header>
  );
}

function UploadPage() {
  const navigate = useNavigate();

  return (
    <div className="app">
      <Header />
      <div className="app-content">
        <Upload onComplete={(videoId) => navigate(`/${videoId}`)} />
      </div>
    </div>
  );
}

function PlayerPage() {
  const { videoId } = useParams<{ videoId: string }>();
  const navigate = useNavigate();

  if (!videoId) return null;

  return (
    <div className="app">
      <Header />
      <div className="app-content">
        <VideoPlayer videoId={videoId} />
        <div className="player-actions">
          <button className="new-upload-btn" onClick={() => navigate('/')}>
            ← Upload another video
          </button>
        </div>
      </div>
    </div>
  );
}

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<UploadPage />} />
        <Route path="/:videoId" element={<PlayerPage />} />
      </Routes>
    </BrowserRouter>
  );
}
