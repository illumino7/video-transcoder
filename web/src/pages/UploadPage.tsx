import { useNavigate } from 'react-router-dom';
import Header from '../components/Header';
import Upload from '../components/Upload';

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
