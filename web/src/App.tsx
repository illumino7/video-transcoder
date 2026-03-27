import { BrowserRouter, Routes, Route } from 'react-router-dom';
import UploadPage from './pages/UploadPage';
import PlayerPage from './pages/PlayerPage';

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<UploadPage />} />
        <Route path="/video/:videoId" element={<PlayerPage />} />
      </Routes>
    </BrowserRouter>
  );
}
