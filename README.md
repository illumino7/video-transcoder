<div align="center">
  <h1>Adaaptiv - Video Transcoder</h1>
  <p>An adaptive video streaming platform for uploading, transcoding, and streaming videos using HLS.</p>

  <p>
    <img src="https://img.shields.io/badge/Go-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go" />
    <img src="https://img.shields.io/badge/React-61DAFB?style=for-the-badge&logo=react&logoColor=black" alt="React" />
    <img src="https://img.shields.io/badge/Tailwind_CSS-06B6D4?style=for-the-badge&logo=tailwindcss&logoColor=white" alt="Tailwind" />
    <img src="https://img.shields.io/badge/Valkey_Streams-FF6600?style=for-the-badge&logo=redis&logoColor=white" alt="Valkey" />
    <img src="https://img.shields.io/badge/PostgreSQL-4169E1?style=for-the-badge&logo=postgresql&logoColor=white" alt="PostgreSQL" />
    <img src="https://img.shields.io/badge/FFmpeg-007808?style=for-the-badge&logo=ffmpeg&logoColor=white" alt="FFmpeg" />
    <img src="https://img.shields.io/badge/MinIO-C72E49?style=for-the-badge&logo=minio&logoColor=white" alt="MinIO" />
    <img src="https://img.shields.io/badge/Docker-2496ED?style=for-the-badge&logo=docker&logoColor=white" alt="Docker" />
  </p>
</div>

---

## Screenshots

<p align="center">
  <img src="assets/Homepage.png" width="100%" alt="Homepage" />
</p>

<p align="center">
  <img src="assets/Upload_With_Preview.png" width="100%" alt="Upload with Preview" />
</p>

<p align="center">
  <img src="assets/Transcode_Client_Progress.png" width="100%" alt="Transcoding Progress" />
</p>

<p align="center">
  <img src="assets/Player_Page.png" width="100%" alt="Video Player Page" />
</p>

<p align="center">
  <img src="assets/Quality_Selector.png" width="100%" alt="Quality Selector" />
</p>

---

## Features

* **Presigned Uploads** - Direct-to-S3 uploads authorized via Go backend endpoints. Video bytes bypass the API server, saving bandwidth.
* **Transactional Outbox Pattern** - Prevents dual-write issues. Video updates and transcoding tasks are committed inside a single Postgres transaction, with an active-leader outbox relay forwarding tasks to Valkey.
* **Valkey Streams & Consumer Groups** - At-least-once task delivery. Workers check out jobs concurrently, tracking pending messages and recovering crashed workers upon restart.
* **Dead Letter Queue (DLQ)** - A background janitor script claims abandoned pending tasks, increments retry counts, and pushes poisoned tasks to a DLQ after multiple failures.
* **Dockerized FFmpeg** - Self-contained worker containers transcode raw files into adaptive multi-bitrate HLS variants (360p, 480p, 720p) using H.265/HEVC packaging.
* **Real-Time SSE Updates** - Server-Sent Events powered by Valkey Pub/Sub notify clients of real-time worker progress.
* **Automated Migrations & Health Checks** - Docker Compose automatically checks database health and executes database schema upgrades before booting application containers.

## System Architecture & Flow

1. **Upload Request**: The client requests a presigned upload URL from the API. The API signs a direct-to-S3 target key and returns it.
2. **Direct Ingest**: The client uploads the raw video file directly to the S3 bucket using the presigned URL.
3. **Transaction Commit (Outbox)**: On upload completion, the client confirms the upload to the API. The API records the video metadata and inserts a task event into the PostgreSQL database under a single transaction (Transactional Outbox).
4. **Relay & Queueing**: A leader-elected outbox publisher polls the database for pending events and pushes them onto the Valkey transcode stream. Once published, the database events are cleaned up.
5. **Transcoding Pipeline**: Worker instances consume from the Valkey stream, download the raw video, extract audio, and run parallel FFmpeg processes to compile AAC audio and H.265/HEVC video into HLS renditions.
6. **Streaming & Updates**: Transcoded segments are uploaded to the streaming bucket, the raw file is deleted, and the task status update is broadcast to the client via Server-Sent Events (SSE).

---

## Tech Stack

| Layer | Technology |
|-------|-----------|
| **Frontend** | React 19, Tailwind CSS v4, Video.js, Vite 7 |
| **API Server** | Go, Chi Router, S3 SDK (Presign API) |
| **Database** | PostgreSQL 16 (Transactional Outbox Pattern) |
| **Queue & Cache** | Valkey 1.0 (Streams, Pub/Sub, Consumer Groups) |
| **Worker** | Go, FFmpeg (libx265, AAC, fMP4 HLS) |
| **Storage** | MinIO (S3-compatible bucket) |
| **Infrastructure** | Docker, Docker Compose, golang-migrate |

---

## Performance Benchmarks

Empirical performance measurements collected on dedicated hardware (AMD Ryzen 7 4800H, 16 vCPUs):

### 1. Database Outbox Writes (`CreateWithOutbox`)
Tested concurrently using Go's parallel benchmarking framework against the local PostgreSQL engine:
* **Throughput**: **1,143 database transactions/second** (equivalent to **2,286 SQL row writes/second**).
* **Latency**: **0.48 ms** transaction execution latency.

### 2. H.265/HEVC Transcoding Storage Efficiency
Comparison of 3 HLS variants (360p, 480p, 720p) on a high-complexity test video:
* **Target Bitrates (Production)**: H.265 generated a **17.78% size reduction** compared to H.264.
* **Equivalent Quality (CRF 23 vs 28)**: H.265 achieved a **46.74% storage footprint reduction** compared to H.264.

---

## Project Structure

```
.
├── cmd/
│   ├── api/                    # API server
│   │   ├── main.go             # Server initialization
│   │   ├── api.go              # Routes and middleware
│   │   ├── upload.go           # Presign + upload verification
│   │   ├── ssestatusHandler.go # SSE via Valkey Pub/Sub
│   │   └── Dockerfile
│   ├── outbox/                 # Outbox relay (leader elected via advisory lock)
│   │   ├── main.go             # Polling publisher relay
│   │   └── Dockerfile
│   └── worker/                 # Stream consumer + DLQ janitor
│       ├── main.go             # Worker initialization
│       ├── config.go           # Consumer loop + janitor
│       ├── handler.go          # Transcode pipeline
│       └── Dockerfile
├── internal/
│   ├── db/                     # DB client, storage models & benchmarks
│   │   ├── db.go               # Pool configuration
│   │   ├── videos.go           # Videos operations
│   │   ├── outbox.go           # Outbox list/delete
│   │   └── videos_test.go      # Go outbox benchmarks
│   ├── env/                    # Environment variable helpers
│   ├── queue/                  # Stream constants + XADD
│   └── storage/                # MinIO client + bucket policies
├── web/
│   └── src/
│       ├── pages/              # UploadPage, PlayerPage
│       ├── components/         # Header, Upload, VideoPlayer
│       ├── config/             # Constants (API base, MinIO URL)
│       ├── App.tsx             # Router
│       └── index.css           # Tailwind v4 theme
├── scripts/                    # FFmpeg transcode & benchmark scripts
├── docker-compose.yaml
└── README.md
```

---

## Getting Started

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/) & [Docker Compose](https://docs.docker.com/compose/install/)
- [Go 1.26+](https://go.dev/dl/) (for local development)
- [Node.js 22+](https://nodejs.org/) (for frontend development)

### Quick Start (Docker Compose)

```bash
git clone https://github.com/illumino7/video-transcoder.git
cd video-transcoder
docker compose up --build
```

| Service | URL |
|---------|-----|
| Frontend | [http://localhost:5173](http://localhost:5173) |
| API Server | [http://localhost:3030](http://localhost:3030) |
| MinIO Console | [http://localhost:9001](http://localhost:9001) |
| Redis Commander | [http://localhost:8081](http://localhost:8081) |

### Local Development

```bash
# Terminal 1 - Infrastructure
docker compose up valkey minio db migrate

# Terminal 2 - API Server
go run ./cmd/api

# Terminal 3 - Outbox Relay
go run ./cmd/outbox

# Terminal 4 - Worker
go run ./cmd/worker

# Terminal 5 - Frontend
cd web && npm install && npm run dev
```

---

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `ADDR` | `:3030` | API server listen address |
| `REDIS_ADDR` | `localhost:6379` | Valkey connection address |
| `DB_DSN` | `postgres://postgres:postgres@localhost:5432/transcoder?sslmode=disable` | PostgreSQL database connection DSN |
| `MINIO_ENDPOINT` | `localhost:9000` | MinIO S3 endpoint |
| `MINIO_ACCESS_KEY` | `minioadmin` | MinIO access key |
| `MINIO_SECRET_KEY` | `minioadmin` | MinIO secret key |
| `MINIO_USE_SSL` | `false` | Enable TLS for MinIO |
| `WORKER_CONCURRENCY` | `2` | Number of concurrent worker goroutines |

---

## License

This project is for educational and portfolio purposes.
