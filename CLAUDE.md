# video-editor-mcp — CLAUDE.md

## What this is
MCP server for video editing and transcription. Go/FFmpeg backend with a Node.js MCP adapter. Runs on Windows desktop with RTX 4090 GPU for fast Whisper transcription.

Part of the Nokemo AI creative suite alongside ComfyUI MCP and the upcoming image-editor MCP.

## Architecture
```
Node.js MCP adapter (mcp-server/, port 8090)
  ↕ JSON-RPC over streamable HTTP
Go API server (cmd/server/)
  ↕ FFmpeg + Whisper
Media files on disk
```

## Project structure
```
video-editor-mcp/
├── cmd/server/          ← Go server entry point
├── internal/
│   ├── api/             ← HTTP handlers
│   ├── config/          ← Configuration
│   ├── engine/          ← FFmpeg operations
│   ├── model/           ← Data models
│   ├── queue/           ← Job queue
│   ├── render/          ← Video rendering pipeline
│   └── storage/         ← File storage
├── mcp-server/          ← Node.js MCP adapter
│   ├── index.js         ← MCP tool definitions
│   └── package.json
├── docker/
│   ├── Dockerfile       ← CPU build (for CI/registry)
│   └── Dockerfile.gpu   ← GPU build (for local RTX 4090)
└── docker-compose.yml   ← Local dev with GPU support
```

## Docker
- **Local (GPU):** `docker compose up -d` — uses Dockerfile.gpu with NVIDIA runtime
- **CI/Registry:** GitHub Actions builds from Dockerfile (CPU) → ghcr.io/isleprince/video-editor-mcp:latest
- **Watchtower** on Windows desktop auto-pulls new images

## Key capabilities
- Video cutting, trimming, concatenation (FFmpeg)
- Whisper transcription (GPU-accelerated with RTX 4090)
- VTT/SRT subtitle generation
- Video rendering and composition

## Integration with Nokemo
- **Host:** 192.168.27.10:8090 (LAN only, Windows desktop)
- **Connected to:** OpenClaw (Nokemo agent) and Claude Code as MCP server
- **Primary workflow:** Zoom recording → transcribe → highlight reel generation
- Only available when Windows desktop is on

## Tech stack
- Go 1.22 (backend API + FFmpeg orchestration)
- Node.js (MCP adapter layer)
- FFmpeg (video processing)
- OpenAI Whisper (transcription)
- Docker with NVIDIA GPU support

## Building
```bash
# Go backend
go mod tidy && go build -o video-editor ./cmd/server

# MCP adapter
cd mcp-server && npm install

# Docker (GPU)
docker compose up -d --build
```
