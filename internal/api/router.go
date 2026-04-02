package api

import (
	"video-editor/internal/queue"
	"video-editor/internal/storage"

	"github.com/go-chi/chi/v5"
)

type Server struct {
	store        storage.Storage
	renderQueue  queue.Queue
	mediaPath    string
	whisperModel string
	enableGPU    bool
}

func NewRouter(store storage.Storage, rq queue.Queue, mediaPath, whisperModel string, enableGPU bool) *chi.Mux {
	s := &Server{store: store, renderQueue: rq, mediaPath: mediaPath, whisperModel: whisperModel, enableGPU: enableGPU}
	r := chi.NewRouter()

	r.Use(requestIDMiddleware)
	r.Use(loggingMiddleware)
	r.Use(corsMiddleware)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", s.handleHealth)
		r.Get("/capabilities", s.handleCapabilities)
		r.Get("/render-profiles", s.handleListRenderProfiles)
		r.Get("/media", s.handleListMediaFiles)

		// Projects
		r.Post("/projects", s.handleCreateProject)
		r.Get("/projects", s.handleListProjects)
		r.Route("/projects/{id}", func(r chi.Router) {
			r.Get("/", s.handleGetProject)
			r.Patch("/", s.handleUpdateProject)
			r.Delete("/", s.handleDeleteProject)

			// Assets
			r.Post("/assets", s.handleImportAsset)
			r.Get("/assets", s.handleListAssets)
			r.Route("/assets/{assetID}", func(r chi.Router) {
				r.Get("/", s.handleGetAsset)
				r.Delete("/", s.handleDeleteAsset)
				r.Get("/transcript", s.handleReadTranscript)
				r.Get("/frame", s.handlePreviewFrame)
				r.Get("/suggest-clips", s.handleSuggestClips)
			})

			// Thumbnails & Visualizations
			r.Get("/thumbnails/{filename}", s.handleServeThumbnail)
			r.Get("/timeline.png", s.handleTimelineImage)

			// Sequences
			r.Post("/sequences", s.handleCreateSequence)
			r.Post("/sequences/batch", s.handleCreateSequencesBatch)
			r.Get("/sequences", s.handleListSequences)
			r.Route("/sequences/{seqID}", func(r chi.Router) {
				r.Get("/", s.handleGetSequence)
				r.Patch("/", s.handleUpdateSequence)
				r.Delete("/", s.handleDeleteSequence)

				// Thumbnails & Visualizations
				r.Post("/thumbnails", s.handleGenerateThumbnails)
				r.Get("/timeline.png", s.handleTimelineImage)
				r.Get("/storyboard.png", s.handleStoryboardImage)

				// Overlays
				r.Post("/overlays", s.handleAddOverlay)
				r.Delete("/overlays/{overlayID}", s.handleDeleteOverlay)

				// Text Overlays
				r.Post("/text-overlays", s.handleAddTextOverlay)
				r.Delete("/text-overlays/{overlayID}", s.handleDeleteTextOverlay)

				// Subtitle Style
				r.Put("/subtitle-style", s.handleSetSubtitleStyle)

				// Tracks
				r.Post("/tracks", s.handleCreateTrack)
				r.Route("/tracks/{trackID}", func(r chi.Router) {
					r.Patch("/", s.handleUpdateTrack)
					r.Delete("/", s.handleDeleteTrack)

					// Clips
					r.Post("/clips", s.handleCreateClip)
					r.Post("/clips/batch", s.handleAddClipsBatch)
					r.Route("/clips/{clipID}", func(r chi.Router) {
						r.Patch("/", s.handleUpdateClip)
						r.Delete("/", s.handleDeleteClip)
					})
				})
			})

			// Renders
			r.Post("/renders", s.handleSubmitRender)
			r.Get("/renders", s.handleListRenders)
			r.Route("/renders/{renderID}", func(r chi.Router) {
				r.Get("/", s.handleGetRender)
				r.Get("/download", s.handleDownloadRender)
				r.Post("/copy-to-media", s.handleCopyRenderToMedia)
			})

			// Transcriptions
			r.Post("/transcriptions", s.handleTranscribe)
			r.Get("/transcriptions", s.handleListTranscriptions)
			r.Get("/transcriptions/{jobID}", s.handleGetTranscription)
		})
	})

	return r
}
