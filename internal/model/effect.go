package model

type InterpolationType string

const (
	InterpolationLinear  InterpolationType = "linear"
	InterpolationHold    InterpolationType = "hold"
	InterpolationEaseIn  InterpolationType = "ease_in"
	InterpolationEaseOut InterpolationType = "ease_out"
	InterpolationBezier  InterpolationType = "bezier"
)

type Keyframe struct {
	Time          float64           `json:"time"`
	Value         float64           `json:"value"`
	Interpolation InterpolationType `json:"interpolation"`
}

type EffectParam struct {
	Name      string     `json:"name"`
	Value     float64    `json:"value"`
	Keyframes []Keyframe `json:"keyframes,omitempty"`
}

type Effect struct {
	ID      string        `json:"id"`
	Type    string        `json:"type"`
	Enabled bool          `json:"enabled"`
	Order   int           `json:"order"`
	Params  []EffectParam `json:"params,omitempty"`
}
