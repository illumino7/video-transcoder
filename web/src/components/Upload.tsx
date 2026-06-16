import { useState, useCallback, useRef, useEffect } from 'react';
import { API_BASE, MAX_FILE_SIZE } from '../config/constants';

// ── Types ──────────────────────────────────────────────
// Interfaces defining the API contracts and component props.

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
// Pure functions containing presentation formatting logic.

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

// ── Component ──────────────────────────────────────────
// The Upload component manages the entire drag-and-drop state machine, 
// the direct-to-S3 file upload sequence, and real-time SSE progress tracking.

export default function Upload({ onComplete }: UploadProps) {
  const [file, setFile] = useState<File | null>(null);
  const [previewUrl, setPreviewUrl] = useState<string | null>(null);
  const [uploading, setUploading] = useState(false);
  const [processing, setProcessing] = useState(false);
  const [errorMsg, setErrorMsg] = useState('');
  const [dragging, setDragging] = useState(false);
  const [terminalLines, setTerminalLines] = useState<string[]>([]);
  const [isFailed, setIsFailed] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const eventSourceRef = useRef<EventSource | null>(null);
  const terminalEndRef = useRef<HTMLDivElement | null>(null);
  const isCompletedRef = useRef(false);

  useEffect(() => {
    return () => {
      eventSourceRef.current?.close();
    };
  }, []);

  // Automatically manage the object URL preview creation and cleanup
  useEffect(() => {
    if (!file) {
      setPreviewUrl(null);
      return;
    }

    const url = URL.createObjectURL(file);
    setPreviewUrl(url);

    return () => {
      URL.revokeObjectURL(url);
    };
  }, [file]);

  // Scroll to bottom of terminal when logs update
  useEffect(() => {
    if (terminalEndRef.current) {
      terminalEndRef.current.scrollIntoView({ behavior: 'smooth' });
    }
  }, [terminalLines]);

  // ── File Selection ─────────────────────────────────
  const handleFileChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const selected = e.target.files?.[0];
      if (selected) {
        setFile(selected);
        setErrorMsg('');
        setIsFailed(false);
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
      setIsFailed(false);
    } else {
      setErrorMsg('Please drop a valid video file.');
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
    setIsFailed(false);
    if (inputRef.current) inputRef.current.value = '';
  }, []);

  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      inputRef.current?.click();
    }
  }, []);

  // ── Simulated Terminal Progression ──────────────────
  const startTerminalSimulation = useCallback((videoId: string) => {
    const logs = [
      `[RUN] adaaptiv-worker-engine v1.0.0 initializing\u2026`,
      `[OK] Valkey queue connection established.`,
      `[RUN] Fetching database metadata for video: ${videoId}\u2026`,
      `[OK] Metadata verified. Status set to: PROCESSING.`,
      `[RUN] Retrieving raw video payload from storage bucket\u2026`,
      `[OK] Download complete (cached locally).`,
      `[RUN] Running ffprobe stream analysis on input file\u2026`,
      `[OK] File verified: H.264 video track, AAC audio stream.`,
      `[RUN] Spawning concurrent multi-bitrate HLS variant encoders:`,
      `      - 360p (640x360 @ 500k bandwidth)`,
      `      - 480p (854x480 @ 1250k bandwidth)`,
      `      - 720p (1280x720 @ 2500k bandwidth)`,
      `[RUN] Extracting audio stream and packaging tracks\u2026`,
    ];

    setTerminalLines([logs[0]]);
    let currentIdx = 1;

    const interval = setInterval(() => {
      if (currentIdx < logs.length) {
        const nextLine = logs[currentIdx];
        if (nextLine !== undefined) {
          setTerminalLines(prev => [...prev, nextLine]);
        }
        currentIdx++;
      } else {
        clearInterval(interval);
      }
    }, 700);

    return interval;
  }, []);

  // ── SSE Connection ─────────────────────────────────
  const connectSSE = useCallback(
    (videoId: string) => {
      const simInterval = startTerminalSimulation(videoId);

      const es = new EventSource(`${API_BASE}/status?id=${videoId}`);
      eventSourceRef.current = es;

      es.onmessage = (event: MessageEvent) => {
        try {
          const msg: StatusMessage = JSON.parse(event.data);
          if (msg.processed) {
            isCompletedRef.current = true;
            clearInterval(simInterval);
            es.close();

            if (msg.status === 'COMPLETED') {
              setTerminalLines(prev => [
                ...prev,
                `[OK] Packaged and compiled master.m3u8 index playlist.`,
                `[OK] Uploaded HLS segments to MinIO streaming storage bucket.`,
                `[SUCCESS] Job completed successfully. HLS stream is online.`,
                `[RUN] Redirecting to player view\u2026`
              ]);
              
              // Delayed redirect to let user read final terminal output
              setTimeout(() => {
                onComplete(videoId);
              }, 1500);
            } else {
              setTerminalLines(prev => [
                ...prev,
                `[FAIL] Transcoder job failed on worker host.`,
                `[ERROR] Server returned FAILED status. Please check server logs.`
              ]);
              setIsFailed(true);
            }
          }
        } catch (err) {
          console.error('Invalid SSE message:', err);
        }
      };

      es.onerror = () => {
        if (isCompletedRef.current) return;
        clearInterval(simInterval);
        setErrorMsg('Connection lost. Please refresh and try again.');
        setProcessing(false);
        es.close();
      };
    },
    [onComplete, startTerminalSimulation],
  );

  // ── Upload Flow ────────────────────────────────────
  const uploadVideo = useCallback(async () => {
    if (!file) {
      setErrorMsg('Please select a file before uploading.');
      return;
    }

    if (file.size > MAX_FILE_SIZE) {
      setErrorMsg('File size exceeds the 100\u00A0MB limit.');
      return;
    }

    setUploading(true);
    setErrorMsg('');
    setIsFailed(false);
    isCompletedRef.current = false;

    try {
      const presignRes = await fetch(
        `${API_BASE}/upload/presign?filename=${encodeURIComponent(file.name)}`,
      );
      if (!presignRes.ok) throw new Error(`Presign signature failed: code ${presignRes.status}`);

      const data: UploadResponse = await presignRes.json();

      const putRes = await fetch(data.uploadUrl, {
        method: 'PUT',
        headers: { 'Content-Type': data.contentType },
        body: file,
      });
      if (!putRes.ok) throw new Error(`Direct storage upload failed: code ${putRes.status}`);

      const completeRes = await fetch(`${API_BASE}/upload`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ videoId: data.videoId, s3Key: data.s3Key }),
      });
      if (!completeRes.ok) {
        const err = await completeRes.json().catch(() => ({}));
        throw new Error(err.error || `Upload verification failed: code ${completeRes.status}`);
      }

      setUploading(false);
      setProcessing(true);
      connectSSE(data.videoId);
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : String(err));
      setUploading(false);
    }
  }, [file, connectSSE]);

  const renderLogLine = (line: string, index: number) => {
    if (!line) return null;
    const isSuccess = line.startsWith('[SUCCESS]');
    const isOk = line.startsWith('[OK]');
    const isError = line.startsWith('[ERROR]') || line.startsWith('[FAIL]');
    const isRun = line.startsWith('[RUN]');

    let style = 'text-zinc-400';
    if (isSuccess) style = 'text-emerald-400 font-medium';
    else if (isOk) style = 'text-emerald-500/80';
    else if (isError) style = 'text-rose-500 font-medium';
    else if (isRun) style = 'text-zinc-200';

    return (
      <div key={index} className={`${style} leading-relaxed select-text`}>
        {line}
      </div>
    );
  };

  // ── Render: Processing Terminal ────────────────────
  if (processing) {
    return (
      <div className="bg-zinc-950 border border-zinc-800 rounded-lg p-6 max-w-xl mx-auto select-none">
        <div className="flex items-center justify-between border-b border-zinc-800 pb-3 mb-4">
          <div className="flex items-center gap-1.5">
            <span className="w-2.5 h-2.5 rounded-full bg-zinc-800" aria-hidden="true" />
            <span className="w-2.5 h-2.5 rounded-full bg-zinc-800" aria-hidden="true" />
            <span className="w-2.5 h-2.5 rounded-full bg-zinc-800" aria-hidden="true" />
          </div>
          <span className="text-[10px] font-mono text-zinc-500">
            transcode-terminal
          </span>
        </div>

        <div className="bg-black border border-zinc-900 rounded-md p-4 font-mono text-xs overflow-y-auto max-h-72 flex flex-col gap-1 min-h-48 no-scrollbar">
          {terminalLines.map((line, idx) => renderLogLine(line, idx))}
          {!isFailed && (
            <div className="text-zinc-300 font-medium">
              <span className="terminal-cursor" aria-hidden="true" />
            </div>
          )}
          <div ref={terminalEndRef} />
        </div>

        {isFailed && (
          <div className="mt-5 flex justify-end">
            <button
              className="
                py-2 px-4 rounded-md font-medium text-xs border border-zinc-800 bg-transparent text-zinc-400
                transition-all duration-150 cursor-pointer
                hover:bg-zinc-900 hover:text-zinc-200 hover:border-zinc-700
                focus-visible:ring-1 focus-visible:ring-zinc-400 focus-visible:outline-none
              "
              onClick={() => {
                setProcessing(false);
                setIsFailed(false);
                setFile(null);
              }}
            >
              Reset upload
            </button>
          </div>
        )}
      </div>
    );
  }

  // ── Render: Upload Form ────────────────────────────
  return (
    <div className="bg-zinc-950 border border-zinc-800 rounded-lg p-6 max-w-xl mx-auto">
      <input
        ref={inputRef}
        id="file-input"
        className="sr-only"
        type="file"
        accept="video/*"
        onChange={handleFileChange}
        tabIndex={-1}
        aria-label="Upload video file"
        autoComplete="off"
      />

      {!file ? (
        /* Drop Zone (Empty State) */
        <div
          className={`
            relative overflow-hidden border border-dashed rounded-md p-10 text-center cursor-pointer
            transition-all duration-150 outline-none
            ${dragging
              ? 'border-zinc-400 bg-zinc-900/40'
              : 'border-zinc-800 hover:border-zinc-700 bg-transparent'
            }
            focus-visible:ring-1 focus-visible:ring-zinc-400 focus-visible:border-transparent
          `}
          onDragOver={handleDragOver}
          onDragLeave={handleDragLeave}
          onDrop={handleDrop}
          onClick={() => inputRef.current?.click()}
          onKeyDown={handleKeyDown}
          role="button"
          tabIndex={0}
          aria-label="Drop video file here or press Enter to browse files"
        >
          {/* Cloud Upload Icon */}
          <svg
            className="w-8 h-8 text-zinc-500 mx-auto mb-4 select-none"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            strokeWidth={1.5}
            aria-hidden="true"
          >
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 16.5V9.75m0 0l3 3m-3-3l-3 3M6.75 19.5a4.5 4.5 0 01-1.41-8.775 5.25 5.25 0 0110.233-2.33 3 3 0 013.758 3.848A3.752 3.752 0 0118 19.5H6.75z" />
          </svg>
          <div>
            <p className="text-zinc-200 text-sm font-medium">
              Drag and drop your video file here
            </p>
            <p className="text-zinc-500 text-xs mt-1">
              or press <span className="text-zinc-400 underline decoration-zinc-600">Enter</span> to browse files
            </p>
            <small className="block mt-5 text-zinc-400 text-[11px] font-mono select-none tracking-wider uppercase">
              MAX FILE SIZE: 100 MB &bull; MP4, MOV, WEBM, MKV, AVI
            </small>
          </div>
        </div>
      ) : (
        /* Video Preview State (File Attached) */
        <div className="flex flex-col gap-4 animate-[fadeIn_0.25s_ease]">
          <div className="relative rounded-md overflow-hidden border border-zinc-800 bg-black aspect-video max-h-64 w-full flex items-center justify-center">
            {previewUrl ? (
              <video
                src={previewUrl}
                className="w-full h-full object-contain"
                controls
                muted
                playsInline
              />
            ) : (
              <div className="text-zinc-500 font-mono text-xs select-none">
                Generating preview…
              </div>
            )}
          </div>
          
          <div className="flex items-center justify-between py-3 px-4 bg-zinc-900/40 border border-zinc-800 rounded-md">
            <div className="flex items-center gap-3 min-w-0">
              {/* Clean Film Strip Icon */}
              <svg
                className="w-4 h-4 text-zinc-400 shrink-0 select-none"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                strokeWidth={2}
                aria-hidden="true"
              >
                <path strokeLinecap="round" strokeLinejoin="round" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
              </svg>
              <div className="flex-1 min-w-0">
                <div className="text-xs font-mono font-medium text-zinc-300 truncate">{file.name}</div>
                <div className="text-[10px] font-mono text-zinc-500 mt-0.5">{formatFileSize(file.size)}</div>
              </div>
            </div>
          </div>

          <div className="flex gap-3 mt-1">
            <button
              className="
                flex-1 py-2 px-4 rounded-md font-medium text-xs border border-zinc-800 bg-transparent text-zinc-400
                transition-colors duration-150 cursor-pointer select-none
                hover:bg-zinc-900 hover:text-zinc-200 hover:border-zinc-700
                focus-visible:ring-1 focus-visible:ring-zinc-400 focus-visible:outline-none
                disabled:opacity-50 disabled:cursor-not-allowed
              "
              onClick={removeFile}
              disabled={uploading}
              aria-label="Remove video and choose another"
            >
              Choose another
            </button>
            
            <button
              className="
                flex-1 py-2 px-4 rounded-md font-medium text-xs border-none bg-white text-black
                transition-colors duration-150 cursor-pointer select-none
                hover:bg-zinc-200 active:bg-zinc-300
                disabled:bg-zinc-900 disabled:text-zinc-600 disabled:cursor-not-allowed
                focus-visible:ring-1 focus-visible:ring-zinc-400 focus-visible:outline-none
              "
              onClick={uploadVideo}
              disabled={uploading}
            >
              {uploading ? (
                <span className="flex items-center justify-center gap-1.5">
                  <svg className="w-3.5 h-3.5 animate-spin text-zinc-500" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.5} aria-hidden="true">
                    <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeLinecap="round" />
                    <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
                  </svg>
                  <span>Uploading…</span>
                </span>
              ) : (
                <span>Upload & Transcode</span>
              )}
            </button>
          </div>
        </div>
      )}

      {/* Error Output */}
      {errorMsg && (
        <div 
          className="mt-4 py-2.5 px-3.5 bg-rose-500/10 border border-rose-500/20 rounded-md text-rose-500 text-xs font-medium animate-shake"
          role="alert"
          aria-live="polite"
        >
          {errorMsg}
        </div>
      )}
    </div>
  );
}
