import { BrowserRouter, Routes, Route } from 'react-router-dom';
import UploadPage from './pages/UploadPage';
import PlayerPage from './pages/PlayerPage';

// App is the root application shell. It establishes the client-side router
// and maps top-level URL paths to their respective page components.
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
