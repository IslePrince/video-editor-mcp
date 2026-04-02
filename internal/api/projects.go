package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"video-editor/internal/model"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type createProjectRequest struct {
	Name     string                 `json:"name"`
	Settings *model.ProjectSettings `json:"settings,omitempty"`
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var req createProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	if req.Name == "" {
		badRequest(w, "name is required")
		return
	}

	settings := model.DefaultProjectSettings()
	if req.Settings != nil {
		settings = *req.Settings
	}

	p := &model.Project{
		ID:        uuid.New().String(),
		Name:      req.Name,
		Settings:  settings,
		Version:   1,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := p.Validate(); err != nil {
		badRequest(w, err.Error())
		return
	}

	if err := s.store.CreateProject(p); err != nil {
		internalError(w, "failed to create project")
		return
	}

	writeJSONWithViz(w, http.StatusCreated, p, p.ID, "")
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.store.ListProjects()
	if err != nil {
		internalError(w, "failed to list projects")
		return
	}
	if projects == nil {
		projects = []model.Project{}
	}
	writeJSON(w, http.StatusOK, projects)
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, err := s.store.GetProject(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			notFound(w, "project not found")
			return
		}
		internalError(w, "failed to get project")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

type updateProjectRequest struct {
	Name     *string                `json:"name,omitempty"`
	Settings *model.ProjectSettings `json:"settings,omitempty"`
}

func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, err := s.store.GetProject(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			notFound(w, "project not found")
			return
		}
		internalError(w, "failed to get project")
		return
	}

	var req updateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}

	if req.Name != nil {
		p.Name = *req.Name
	}
	if req.Settings != nil {
		p.Settings = *req.Settings
	}

	if err := p.Validate(); err != nil {
		badRequest(w, err.Error())
		return
	}

	p.Version++
	p.UpdatedAt = time.Now().UTC()

	if err := s.store.UpdateProject(p); err != nil {
		internalError(w, "failed to update project")
		return
	}
	writeJSONWithViz(w, http.StatusOK, p, p.ID, "")
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.store.DeleteProject(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			notFound(w, "project not found")
			return
		}
		internalError(w, "failed to delete project")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
