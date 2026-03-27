import { useState, useCallback, useRef, useEffect } from 'react';
import { API_BASE, MAX_FILE_SIZE } from '../config/constants';

// ── Types ──────────────────────────────────────────────

interface UploadResponse {
  videoId: string;
  uploadUrl: string;
  s3Key: string;
  contentType: string;
}

interface StatusMessage {
  id: string;
  processed: boolean;
  status: string;
}

interface UploadProps {
  onComplete: (videoId: string) => void;
}

// ── Helpers ────────────────────────────────────────────

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

// ── Component ──────────────────────────────────────────

export default function Upload({ onComplete }: UploadProps) {
  const [file, setFile] = useState<File | null>(null);
  const [uploading, setUploading] = useState(false);
  const [processing, setProcessing] = useState(false);
  const [errorMsg, setErrorMsg] = useState('');
  const [dragging, setDragging] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const eventSourceRef = useRef<EventSource | null>(null);

  useEffect(() => {
    return () => {
      eventSourceRef.current?.close();
    };
  }, []);

  // ── File Selection ─────────────────────────────────

  const handleFileChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const selected = e.target.files?.[0];
      if (selected) {
        setFile(selected);
        setErrorMsg('');
      }
    },
    [],
  );

  const handleDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setDragging(false);
    const dropped = e.dataTransfer.files?.[0];
    if (dropped && dropped.type.startsWith('video/')) {
      setFile(dropped);
      setErrorMsg('');
    } else {
      setErrorMsg('Please drop a video file.');
    }
  }, []);

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setDragging(true);
  }, []);

  const handleDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setDragging(false);
  }, []);

  const removeFile = useCallback(() => {
    setFile(null);
    if (inputRef.current) inputRef.current.value = '';
  }, []);

  // ── SSE Connection ─────────────────────────────────

  const connectSSE = useCallback(
    (videoId: string) => {
      const es = new EventSource(`${API_BASE}/status?id=${videoId}`);
      eventSourceRef.current = es;

      es.onmessage = (event: MessageEvent) => {
        try {
          const msg: StatusMessage = JSON.parse(event.data);
          if (msg.processed) {
            setProcessing(false);
            es.close();
            onComplete(videoId);
          }
        } catch (err) {
          console.error('Invalid SSE message:', err);
        }
      };

      es.onerror = () => {
        setErrorMsg('Connection lost. Please refresh and try again.');
        setProcessing(false);
        es.close();
      };
    },
    [onComplete],
  );

  // ── Upload Flow ────────────────────────────────────

  const uploadVideo = useCallback(async () => {
    if (!file) {
      setErrorMsg('Please select a file before uploading.');
      return;
    }

    if (file.size > MAX_FILE_SIZE) {
      setErrorMsg('File exceeds the 100 MB limit.');
      return;
    }

    setUploading(true);
    setErrorMsg('');

    try {
      const presignRes = await fetch(
        `${API_BASE}/upload/presign?filename=${encodeURIComponent(file.name)}`,
      );
      if (!presignRes.ok) throw new Error(`Presign failed: ${presignRes.status}`);

      const data: UploadResponse = await presignRes.json();

      const putRes = await fetch(data.uploadUrl, {
        method: 'PUT',
        headers: { 'Content-Type': data.contentType },
        body: file,
      });
      if (!putRes.ok) throw new Error(`Direct upload failed: ${putRes.status}`);

      const completeRes = await fetch(`${API_BASE}/upload`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ videoId: data.videoId, s3Key: data.s3Key }),
      });
      if (!completeRes.ok) {
        const err = await completeRes.json().catch(() => ({}));
        throw new Error(err.error || `Upload verification failed: ${completeRes.status}`);
      }

      setUploading(false);
      setProcessing(true);
      connectSSE(data.videoId);
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : String(err));
      setUploading(false);
    }
  }, [file, connectSSE]);

  // ── Render: Processing ─────────────────────────────

  if (processing) {
    return (
      <div className="bg-white/[0.06] backdrop-blur-2xl border border-white/[0.08] rounded-3xl p-10 transition-all duration-300 hover:border-indigo-500/40 hover:shadow-[0_0_40px_rgba(99,102,241,0.25)]">
        <div className="text-center py-16 px-8 animate-[fadeIn_0.5s_ease]">
          <div className="w-14 h-14 border-3 border-zinc-700 border-t-accent rounded-full mx-auto mb-6 animate-spin-slow" />
          <h3 className="text-lg font-semibold text-zinc-100 mb-2">Transcoding your video</h3>
          <p className="text-zinc-400 text-sm">
            Encoding multiple quality levels<span className="processing-dots" />
          </p>
        </div>
      </div>
    );
  }

  // ── Render: Upload ─────────────────────────────────

  return (
    <div className="bg-white/[0.06] backdrop-blur-2xl border border-white/[0.08] rounded-3xl p-10 transition-all duration-300 hover:border-indigo-500/40 hover:shadow-[0_0_40px_rgba(99,102,241,0.25)]">
      {/* Drop Zone */}
      <div
        className={`
          relative overflow-hidden border-2 border-dashed rounded-2xl p-12 text-center cursor-pointer
          transition-all duration-300
          ${dragging
            ? 'border-accent bg-indigo-500/5'
            : 'border-white/[0.08] hover:border-accent hover:bg-indigo-500/5'
          }
        `}
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
        onClick={() => inputRef.current?.click()}
        role="button"
        tabIndex={0}
        aria-label="Drop video file here or click to browse"
      >
        <div className="text-4xl mb-4 relative z-10">🎬</div>
        <div className="relative z-10">
          <p className="text-zinc-400 text-[0.95rem]">
            Drag and drop your video here, or{' '}
            <strong className="text-accent-light font-semibold">browse</strong>
          </p>
          <small className="block mt-2 text-zinc-500 text-xs">
            Supports all major video formats • Max 100 MB
          </small>
        </div>
      </div>

      <input
        ref={inputRef}
        id="file-input"
        className="sr-only"
        type="file"
        accept="video/*"
        onChange={handleFileChange}
      />

      {/* File Info */}
      {file && (
        <div className="flex items-center gap-3 mt-5 py-3.5 px-4 bg-white/[0.04] border border-white/[0.08] rounded-xl animate-[fadeIn_0.3s_ease]">
          <span className="text-xl shrink-0">🎥</span>
          <div className="flex-1 min-w-0">
            <div className="text-sm font-medium text-zinc-200 truncate">{file.name}</div>
            <div className="text-xs text-zinc-500">{formatFileSize(file.size)}</div>
          </div>
          <button
            className="bg-transparent border-none text-zinc-500 cursor-pointer p-1 text-lg transition-colors duration-200 shrink-0 hover:text-error"
            onClick={removeFile}
            aria-label="Remove file"
          >
            ✕
          </button>
        </div>
      )}

      {/* Error */}
      {errorMsg && (
        <div className="mt-4 py-3 px-4 bg-red-400/10 border border-red-400/20 rounded-lg text-error text-sm animate-shake">
          {errorMsg}
        </div>
      )}

      {/* Upload Button */}
      <button
        className="
          relative overflow-hidden flex items-center justify-center gap-2 mt-6 w-full py-3.5 px-6
          bg-gradient-to-br from-accent via-accent-purple to-accent-light
          text-white font-semibold text-[0.95rem] border-none rounded-xl cursor-pointer
          transition-all duration-300 active:scale-[0.98]
          disabled:opacity-50 disabled:cursor-not-allowed disabled:active:scale-100
          hover:brightness-110
        "
        onClick={uploadVideo}
        disabled={!file || uploading}
      >
        {uploading ? (
          <>
            <span className="w-[18px] h-[18px] border-2 border-white/30 border-t-white rounded-full animate-spin-slow" />
            Uploading…
          </>
        ) : (
          <>↑ Upload & Transcode</>
        )}
      </button>
    </div>
  );
}
