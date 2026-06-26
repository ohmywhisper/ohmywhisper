package format

type TranscriptionResponse struct {
	Text string `json:"text"`
}

type Word struct {
	Word  string  `json:"word"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

type Segment struct {
	ID               int     `json:"id"`
	Seek             int     `json:"seek"`
	Start            float64 `json:"start"`
	End              float64 `json:"end"`
	Text             string  `json:"text"`
	NoSpeechProb     float64 `json:"no_speech_prob"`
	AvgLogprob       float64 `json:"avg_logprob"`
	CompressionRatio float64 `json:"compression_ratio"`
	Temperature      float64 `json:"temperature"`
	Words            []Word  `json:"words,omitempty"`
}

type VerboseResponse struct {
	Task     string    `json:"task"`
	Language string    `json:"language"`
	Duration float64   `json:"duration"`
	Text     string    `json:"text"`
	Segments []Segment `json:"segments"`
	Words    []Word    `json:"words,omitempty"`
}

type SegmentResponse struct {
	Segments []Segment `json:"segments"`
}

type ModelInfo struct {
	ID           string `json:"id"`
	Object       string `json:"object"`
	Multilingual bool   `json:"multilingual"`
}

type ModelResponse struct {
	Task     string    `json:"task"`
	Language string    `json:"language"`
	Duration float64   `json:"duration"`
	Text     string    `json:"text"`
	Model    ModelInfo `json:"model"`
	Segments []Segment `json:"segments"`
}

type ModelListResponse struct {
	Object string      `json:"object"`
	Data   []ModelInfo `json:"data"`
}
