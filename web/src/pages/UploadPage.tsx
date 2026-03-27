import { useNavigate } from 'react-router-dom';
import Header from '../components/Header';
import Upload from '../components/Upload';

// UploadPage acts as the primary layout wrapper and route entrypoint for the upload flow.
// It isolates the Upload component state from other top-level pages and handles
// the programmatic routing transition once a transcode is fully verified.
export default function UploadPage() {
  const navigate = useNavigate();

  return (
    <div className="flex flex-col items-center min-h-dvh p-8 max-sm:p-6">
      <Header />
      <div className="w-full max-w-[960px] animate-fade-in-up">
        <Upload onComplete={(videoId) => navigate(`/video/${videoId}`)} />
      </div>
    </div>
  );
}
