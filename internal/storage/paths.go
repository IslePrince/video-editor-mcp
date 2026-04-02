package storage

import "path/filepath"

func projectDir(root, projectID string) string {
	return filepath.Join(root, "projects", projectID)
}

func projectFilePath(root, projectID string) string {
	return filepath.Join(projectDir(root, projectID), "project.json")
}

func assetsDir(root, projectID string) string {
	return filepath.Join(projectDir(root, projectID), "assets")
}

func assetDir(root, projectID, assetID string) string {
	return filepath.Join(assetsDir(root, projectID), assetID)
}

func proxiesDir(root, projectID string) string {
	return filepath.Join(projectDir(root, projectID), "proxies")
}

func thumbnailsDir(root, projectID string) string {
	return filepath.Join(projectDir(root, projectID), "thumbnails")
}

func waveformsDir(root, projectID string) string {
	return filepath.Join(projectDir(root, projectID), "waveforms")
}

func rendersDir(root, projectID string) string {
	return filepath.Join(projectDir(root, projectID), "renders")
}

func transcriptionsDir(root, projectID string) string {
	return filepath.Join(projectDir(root, projectID), "transcriptions")
}

func tempDir(root, projectID string) string {
	return filepath.Join(projectDir(root, projectID), "temp")
}
