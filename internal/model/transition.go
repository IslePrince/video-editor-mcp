package model

type TransitionType string

const (
	TransitionCrossfade  TransitionType = "crossfade"
	TransitionWipe       TransitionType = "wipe"
	TransitionDissolve   TransitionType = "dissolve"
	TransitionSlide      TransitionType = "slide"
	TransitionFade       TransitionType = "fade"
	TransitionDipToColor TransitionType = "dip_to_color"
	TransitionNone       TransitionType = "none"
)

type Transition struct {
	ID       string            `json:"id"`
	Type     TransitionType    `json:"type"`
	Duration float64           `json:"duration"`
	Params   map[string]string `json:"params,omitempty"`
}
