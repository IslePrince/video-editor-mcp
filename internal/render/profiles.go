package render

import "video-editor/internal/model"

var DefaultProfiles = map[string]model.RenderProfile{
	"h264_high": {
		Name:       "h264_high",
		Format:     "mp4",
		VideoCodec: "libx264",
		AudioCodec: "aac",
		Quality:    18,
		Passes:     1,
	},
	"h264_medium": {
		Name:       "h264_medium",
		Format:     "mp4",
		VideoCodec: "libx264",
		AudioCodec: "aac",
		Quality:    22,
		Passes:     1,
	},
	"h264_web": {
		Name:       "h264_web",
		Format:     "mp4",
		VideoCodec: "libx264",
		AudioCodec: "aac",
		Width:      1280,
		Height:     720,
		Quality:    23,
		Passes:     1,
	},
	"h265_high": {
		Name:       "h265_high",
		Format:     "mp4",
		VideoCodec: "libx265",
		AudioCodec: "aac",
		Quality:    22,
		Passes:     1,
	},
}

func GetProfile(name string) (model.RenderProfile, bool) {
	p, ok := DefaultProfiles[name]
	return p, ok
}

func ListProfiles() []model.RenderProfile {
	profiles := make([]model.RenderProfile, 0, len(DefaultProfiles))
	for _, p := range DefaultProfiles {
		profiles = append(profiles, p)
	}
	return profiles
}
