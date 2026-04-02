package model

import "fmt"

func (p *Project) Validate() error {
	if p.Name == "" {
		return fmt.Errorf("project name is required")
	}
	return p.Settings.Validate()
}

func (s *ProjectSettings) Validate() error {
	if s.Width <= 0 || s.Height <= 0 {
		return fmt.Errorf("width and height must be positive")
	}
	if s.FrameRate.Num <= 0 || s.FrameRate.Den <= 0 {
		return fmt.Errorf("frame rate numerator and denominator must be positive")
	}
	if s.SampleRate <= 0 {
		return fmt.Errorf("sample rate must be positive")
	}
	return nil
}

func (a *Asset) Validate() error {
	if a.Name == "" {
		return fmt.Errorf("asset name is required")
	}
	if a.FilePath == "" {
		return fmt.Errorf("asset file_path is required")
	}
	switch a.Type {
	case AssetTypeVideo, AssetTypeAudio, AssetTypeImage, AssetTypeSubtitle:
	default:
		return fmt.Errorf("invalid asset type: %s", a.Type)
	}
	return nil
}

func (c *Clip) Validate() error {
	if c.AssetID == "" {
		return fmt.Errorf("clip asset_id is required")
	}
	if c.TimelineOut <= c.TimelineIn {
		return fmt.Errorf("timeline_out must be greater than timeline_in")
	}
	if c.SourceOut <= c.SourceIn {
		return fmt.Errorf("source_out must be greater than source_in")
	}
	if c.Speed <= 0 {
		return fmt.Errorf("speed must be positive")
	}
	return nil
}
