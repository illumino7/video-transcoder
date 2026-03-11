import { useState, useCallback, useRef, useEffect } from 'react';
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

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

export default function Upload({ onComplete }: UploadProps) {
  const [file, setFile] = useState<File | null>(null);
  const [uploading, setUploading] = useState(false);
  const [processing, setProcessing] = useState(false);
  const [errorMsg, setErrorMsg] = useState('');
  const [dragging, setDragging] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const eventSourceRef = useRef<EventSource | null>(null);

  // Cleanup SSE on unmount
  useEffect(() => {
    return () => {
      eventSourceRef.current?.close();
    };
  }, []);

  const handleFileChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    const selected = e.target.files?.[0];
    if (selected) {
      setFile(selected);
      setErrorMsg('');
    }
  }, []);

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

  const connectSSE = useCallback((videoId: string) => {
    const es = new EventSource(`http://localhost:3030/api/v1/status?id=${videoId}`);
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
  }, [onComplete]);

  const uploadVideo = useCallback(async () => {
    if (!file) {
      setErrorMsg('Please select a file before uploading.');
      return;
    }

    setUploading(true);
    setErrorMsg('');

    try {
      // Step 1: Get presigned PUT URL
      const presignRes = await fetch(
        `http://localhost:3030/api/v1/upload/presign?filename=${encodeURIComponent(file.name)}`
      );
      if (!presignRes.ok) throw new Error(`Presign failed: ${presignRes.status}`);

      const data: UploadResponse = await presignRes.json();

      // Step 2: Upload file directly to MinIO via presigned URL
      const putRes = await fetch(data.uploadUrl, {
        method: 'PUT',
        headers: { 'Content-Type': data.contentType },
        body: file,
      });
      if (!putRes.ok) throw new Error(`Direct upload failed: ${putRes.status}`);

      // Step 3: Verify upload + enqueue transcode
      const completeRes = await fetch('http://localhost:3030/api/v1/upload', {
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

  // ── Processing State ──
  if (processing) {
    return (
      <div className="upload-card">
        <div className="processing-card">
          <div className="processing-spinner" />
          <h3>Transcoding your video</h3>
          <p>
            Encoding multiple quality levels<span className="processing-dots" />
          </p>
        </div>
      </div>
    );
  }

  // ── Upload State ──
  return (
    <div className="upload-card">
      <div
        className={`drop-zone${dragging ? ' dragging' : ''}`}
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
        onClick={() => inputRef.current?.click()}
        role="button"
        tabIndex={0}
        aria-label="Drop video file here or click to browse"
      >
        <div className="drop-zone-icon">🎬</div>
        <div className="drop-zone-text">
          <p>
            Drag and drop your video here, or <strong>browse</strong>
          </p>
          <small>Supports all major video formats • Max 100 MB</small>
        </div>
      </div>

      <input
        ref={inputRef}
        id="file-input"
        className="visually-hidden"
        type="file"
        accept="video/*"
        onChange={handleFileChange}
      />

      {file && (
        <div className="file-info">
          <span className="file-info-icon">🎥</span>
          <div className="file-info-details">
            <div className="file-info-name">{file.name}</div>
            <div className="file-info-size">{formatFileSize(file.size)}</div>
          </div>
          <button
            className="file-info-remove"
            onClick={removeFile}
            aria-label="Remove file"
          >
            ✕
          </button>
        </div>
      )}

      {errorMsg && <div className="error-msg">{errorMsg}</div>}

      <button
        className="upload-btn"
        onClick={uploadVideo}
        disabled={!file || uploading}
      >
        {uploading ? (
          <>
            <span className="processing-spinner" style={{ width: 18, height: 18, margin: 0, borderWidth: 2 }} />
            Uploading…
          </>
        ) : (
          <>↑ Upload & Transcode</>
        )}
      </button>
    </div>
  );
}
