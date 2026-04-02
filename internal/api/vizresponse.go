package api

import (
	"encoding/json"
	"net/http"
)

// writeJSONWithViz wraps a response payload with timeline/storyboard visualization URLs.
func writeJSONWithViz(w http.ResponseWriter, status int, v interface{}, projectID string, seqID string) {
	// Marshal original payload to map
	data, err := json.Marshal(v)
	if err != nil {
		writeJSON(w, status, v)
		return
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		// If it's an array or non-object, wrap it
		result = map[string]interface{}{
			"data": v,
		}
	}

	// Add visualization URLs
	if projectID != "" {
		result["timeline_url"] = "/api/v1/projects/" + projectID + "/timeline.png"
		if seqID != "" {
			result["storyboard_url"] = "/api/v1/projects/" + projectID + "/sequences/" + seqID + "/storyboard.png"
			result["timeline_detail_url"] = "/api/v1/projects/" + projectID + "/sequences/" + seqID + "/timeline.png"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(result)
}
