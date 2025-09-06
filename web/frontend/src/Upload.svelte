<script lang="ts">
    import { onDestroy } from "svelte";
    import VideoPlayer from "./VideoPlayer.svelte";

    let file: File | null = null;
    let uploading = false;
    let processed = false;
    let videoId: string | null = null;
    let errorMsg = "";
    let eventSource: EventSource | null = null;

    interface UploadResponse {
        videoId: string;
    }

    interface StatusMessage {
        id: string;
        processed: boolean;
        status: string;
    }

    const handleFileChange = (e: Event) => {
        const target = e.target as HTMLInputElement;
        if (target.files && target.files.length > 0) {
            file = target.files[0];
        }
    };

    const uploadVideo = async () => {
        if (!file) {
            errorMsg = "Please select a file before uploading.";
            return;
        }

        uploading = true;
        errorMsg = "";

        try {
            const formData = new FormData();
            formData.append("video", file);

            const res = await fetch("http://localhost:3030/api/v1/upload", {
                method: "POST",
                body: formData,
            });

            if (!res.ok) {
                throw new Error(`Upload failed: ${res.status}`);
            }

            const data: UploadResponse = await res.json();
            videoId = data.videoId;

            connectSSE(videoId);
        } catch (err: unknown) {
            errorMsg = err instanceof Error ? err.message : String(err);
            uploading = false;
        }
    };

    const connectSSE = (id: string) => {
        eventSource = new EventSource(`http://localhost:3030/api/v1/status?id=${id}`);

        eventSource.onmessage = (event: MessageEvent) => {
            try {
                const msg: StatusMessage = JSON.parse(event.data);
                console.log("Message from server:", msg);

                if (msg.processed) {
                    processed = true;
                    uploading = false;
                    eventSource?.close();
                }
            } catch (e) {
                console.error("Invalid SSE message:", e);
            }
        };

        eventSource.onerror = (err: Event) => {
            console.error("SSE error:", err);
            errorMsg = "Connection error. Please refresh the page and try again.";
            eventSource?.close();
        };
    };

    onDestroy(() => {
        eventSource?.close();
    });
</script>

<div class="upload-container">
    {#if !uploading && !processed}
        <input
            id="file-input"
            class="visually-hidden"
            type="file"
            accept="video/*"
            on:change={handleFileChange}
        />
        <label for="file-input" class="btn">Choose File</label>
        <button class="btn primary" on:click={uploadVideo}>Upload</button>
        {#if file}
            <div class="file-name" title={file.name}>{file.name}</div>
        {/if}
    {/if}

    {#if errorMsg}
        <p style="color:red;">{errorMsg}</p>
    {/if}

    {#if uploading && !processed}
        <div class="processing-center">
            <div class="processing-inner">
                <p class="processing-text">Processing video</p>
                <div class="spinner"></div>
            </div>
        </div>
    {/if}

    {#if processed && videoId}
        <div class="player-center">
            <VideoPlayer {videoId} />
        </div>
    {/if}
</div>

<style>
    .upload-container {
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: 1rem;
    }
    .btn {
        padding: 0.5rem 1rem;
        border: 1px solid #d1d5db;
        background: #fff;
        color: #111;
        border-radius: 6px;
        cursor: pointer;
        user-select: none;
    }
    .btn:hover {
        background: #f3f4f6;
    }
    .btn.primary {
        background: #3b82f6;
        color: #fff;
        border-color: #3b82f6;
    }
    .btn.primary:hover {
        background: #2563eb;
        border-color: #2563eb;
    }
    .visually-hidden {
        position: absolute;
        width: 1px;
        height: 1px;
        padding: 0;
        margin: -1px;
        overflow: hidden;
        clip: rect(0, 0, 0, 0);
        white-space: nowrap;
        border: 0;
    }
    .processing-center {
        display: flex;
        justify-content: center;
        align-items: center;
        min-height: 40vh;
        width: 100%;
    }
    .processing-inner {
        display: flex;
        flex-direction: column;
        align-items: center;
    }
    .processing-text {
        margin-bottom: 0.75rem;
        text-align: center;
    }
    .file-name {
        max-width: 80ch;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
        color: #374151;
        font-size: 0.9rem;
    }
    .player-center {
        display: flex;
        justify-content: center;
        align-items: center;
        min-height: 70vh;
        width: 100%;
    }
    .spinner {
        border: 4px solid #f3f3f3;
        border-top: 4px solid #3b82f6;
        border-radius: 50%;
        width: 36px;
        height: 36px;
        animation: spin 1s linear infinite;
    }
    @keyframes spin {
        to {
            transform: rotate(360deg);
        }
    }
</style>
