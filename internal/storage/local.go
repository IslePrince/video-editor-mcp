package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"video-editor/internal/model"
)

type LocalStorage struct {
	root string
	mu   sync.RWMutex
}

func NewLocalStorage(root string) (*LocalStorage, error) {
	if err := os.MkdirAll(filepath.Join(root, "projects"), 0755); err != nil {
		return nil, fmt.Errorf("create storage root: %w", err)
	}
	return &LocalStorage{root: root}, nil
}

func (s *LocalStorage) EnsureProjectDirs(projectID string) error {
	dirs := []string{
		projectDir(s.root, projectID),
		assetsDir(s.root, projectID),
		proxiesDir(s.root, projectID),
		thumbnailsDir(s.root, projectID),
		waveformsDir(s.root, projectID),
		rendersDir(s.root, projectID),
		transcriptionsDir(s.root, projectID),
		tempDir(s.root, projectID),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("create dir %s: %w", d, err)
		}
	}
	return nil
}

func (s *LocalStorage) AssetOriginalPath(projectID, assetID, ext string) string {
	return filepath.Join(assetDir(s.root, projectID, assetID), "original"+ext)
}

func (s *LocalStorage) RenderOutputPath(projectID, jobID string) string {
	return filepath.Join(rendersDir(s.root, projectID), jobID+".mp4")
}

func (s *LocalStorage) ThumbnailsPath(projectID string) string {
	return thumbnailsDir(s.root, projectID)
}

// Projects

func (s *LocalStorage) CreateProject(p *model.Project) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.EnsureProjectDirs(p.ID); err != nil {
		return err
	}
	return s.saveProject(p)
}

func (s *LocalStorage) GetProject(id string) (*model.Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.loadProject(id)
}

func (s *LocalStorage) ListProjects() ([]model.Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(filepath.Join(s.root, "projects"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var projects []model.Project
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p, err := s.loadProject(e.Name())
		if err != nil {
			continue
		}
		p.Assets = nil
		p.Sequences = nil
		projects = append(projects, *p)
	}
	return projects, nil
}

func (s *LocalStorage) UpdateProject(p *model.Project) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.loadProject(p.ID); err != nil {
		return err
	}
	return s.saveProject(p)
}

func (s *LocalStorage) DeleteProject(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := projectDir(s.root, id)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("project not found: %s", id)
	}
	return os.RemoveAll(dir)
}

// Assets

func (s *LocalStorage) CreateAsset(projectID string, a *model.Asset) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, err := s.loadProject(projectID)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(assetDir(s.root, projectID, a.ID), 0755); err != nil {
		return err
	}

	p.Assets = append(p.Assets, *a)
	p.Version++
	return s.saveProject(p)
}

func (s *LocalStorage) GetAsset(projectID, assetID string) (*model.Asset, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, err := s.loadProject(projectID)
	if err != nil {
		return nil, err
	}
	for i := range p.Assets {
		if p.Assets[i].ID == assetID {
			return &p.Assets[i], nil
		}
	}
	return nil, fmt.Errorf("asset not found: %s", assetID)
}

func (s *LocalStorage) ListAssets(projectID string, assetType *model.AssetType) ([]model.Asset, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, err := s.loadProject(projectID)
	if err != nil {
		return nil, err
	}
	if assetType == nil {
		return p.Assets, nil
	}
	var filtered []model.Asset
	for _, a := range p.Assets {
		if a.Type == *assetType {
			filtered = append(filtered, a)
		}
	}
	return filtered, nil
}

func (s *LocalStorage) DeleteAsset(projectID, assetID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, err := s.loadProject(projectID)
	if err != nil {
		return err
	}

	found := false
	assets := make([]model.Asset, 0, len(p.Assets))
	for _, a := range p.Assets {
		if a.ID == assetID {
			found = true
			continue
		}
		assets = append(assets, a)
	}
	if !found {
		return fmt.Errorf("asset not found: %s", assetID)
	}

	os.RemoveAll(assetDir(s.root, projectID, assetID))

	p.Assets = assets
	p.Version++
	return s.saveProject(p)
}

func (s *LocalStorage) UpdateAsset(projectID string, a *model.Asset) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, err := s.loadProject(projectID)
	if err != nil {
		return err
	}
	for i := range p.Assets {
		if p.Assets[i].ID == a.ID {
			p.Assets[i] = *a
			p.Version++
			return s.saveProject(p)
		}
	}
	return fmt.Errorf("asset not found: %s", a.ID)
}

// Sequences (stored inside project.json)

func (s *LocalStorage) CreateSequence(projectID string, seq *model.Sequence) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, err := s.loadProject(projectID)
	if err != nil {
		return err
	}
	p.Sequences = append(p.Sequences, *seq)
	p.Version++
	return s.saveProject(p)
}

func (s *LocalStorage) GetSequence(projectID, seqID string) (*model.Sequence, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, err := s.loadProject(projectID)
	if err != nil {
		return nil, err
	}
	for i := range p.Sequences {
		if p.Sequences[i].ID == seqID {
			return &p.Sequences[i], nil
		}
	}
	return nil, fmt.Errorf("sequence not found: %s", seqID)
}

func (s *LocalStorage) ListSequences(projectID string) ([]model.Sequence, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, err := s.loadProject(projectID)
	if err != nil {
		return nil, err
	}
	return p.Sequences, nil
}

func (s *LocalStorage) UpdateSequence(projectID string, seq *model.Sequence) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, err := s.loadProject(projectID)
	if err != nil {
		return err
	}
	for i := range p.Sequences {
		if p.Sequences[i].ID == seq.ID {
			p.Sequences[i] = *seq
			p.Version++
			return s.saveProject(p)
		}
	}
	return fmt.Errorf("sequence not found: %s", seq.ID)
}

func (s *LocalStorage) DeleteSequence(projectID, seqID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, err := s.loadProject(projectID)
	if err != nil {
		return err
	}
	found := false
	seqs := make([]model.Sequence, 0, len(p.Sequences))
	for _, sq := range p.Sequences {
		if sq.ID == seqID {
			found = true
			continue
		}
		seqs = append(seqs, sq)
	}
	if !found {
		return fmt.Errorf("sequence not found: %s", seqID)
	}
	p.Sequences = seqs
	p.Version++
	return s.saveProject(p)
}

// Render Jobs (stored as individual JSON files)

func (s *LocalStorage) SaveRenderJob(job *model.RenderJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := rendersDir(s.root, job.ProjectID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, job.ID+".json"), data, 0644)
}

func (s *LocalStorage) GetRenderJob(projectID, jobID string) (*model.RenderJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	path := filepath.Join(rendersDir(s.root, projectID), jobID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("render job not found: %s", jobID)
		}
		return nil, err
	}
	var job model.RenderJob
	if err := json.Unmarshal(data, &job); err != nil {
		return nil, err
	}
	return &job, nil
}

func (s *LocalStorage) ListRenderJobs(projectID string) ([]model.RenderJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := rendersDir(s.root, projectID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var jobs []model.RenderJob
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var job model.RenderJob
		if err := json.Unmarshal(data, &job); err != nil {
			continue
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

// Transcription Jobs (stored as individual JSON files)

func (s *LocalStorage) TranscriptionsPath(projectID string) string {
	return transcriptionsDir(s.root, projectID)
}

func (s *LocalStorage) SaveTranscriptionJob(job *model.TranscriptionJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := transcriptionsDir(s.root, job.ProjectID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, job.ID+".json"), data, 0644)
}

func (s *LocalStorage) GetTranscriptionJob(projectID, jobID string) (*model.TranscriptionJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	path := filepath.Join(transcriptionsDir(s.root, projectID), jobID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("transcription job not found: %s", jobID)
		}
		return nil, err
	}
	var job model.TranscriptionJob
	if err := json.Unmarshal(data, &job); err != nil {
		return nil, err
	}
	return &job, nil
}

func (s *LocalStorage) ListTranscriptionJobs(projectID string) ([]model.TranscriptionJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := transcriptionsDir(s.root, projectID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var jobs []model.TranscriptionJob
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var job model.TranscriptionJob
		if err := json.Unmarshal(data, &job); err != nil {
			continue
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

// Internal helpers

func (s *LocalStorage) saveProject(p *model.Project) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal project: %w", err)
	}
	path := projectFilePath(s.root, p.ID)
	return os.WriteFile(path, data, 0644)
}

func (s *LocalStorage) loadProject(id string) (*model.Project, error) {
	path := projectFilePath(s.root, id)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("project not found: %s", id)
		}
		return nil, fmt.Errorf("read project: %w", err)
	}
	var p model.Project
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("unmarshal project: %w", err)
	}
	return &p, nil
}
