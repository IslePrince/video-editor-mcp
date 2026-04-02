package main

import (
	"fmt"
	"log"
	"net/http"

	"video-editor/internal/api"
	"video-editor/internal/config"
	"video-editor/internal/queue"
	"video-editor/internal/render"
	"video-editor/internal/storage"
)

func main() {
	cfg := config.Load()

	store, err := storage.NewLocalStorage(cfg.StorageRoot)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}

	// Job queue
	rq := queue.NewMemoryQueue(100)

	// Worker pool
	pool := render.NewWorkerPool(rq, cfg.WorkerCount)
	pool.Start()
	defer pool.Stop()

	router := api.NewRouter(store, rq, cfg.MediaPath, cfg.WhisperModel, cfg.EnableGPU)

	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("Video Editor API starting on %s", addr)
	log.Printf("Storage root: %s", cfg.StorageRoot)
	log.Printf("Worker count: %d", cfg.WorkerCount)

	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
