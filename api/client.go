package api

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/gin-gonic/gin"
	whisperlib "ohmywhisper/api/whisper"
	"ohmywhisper/format"
)

type Client struct {
	engine *whisperlib.Engine
}

func NewClient(e *whisperlib.Engine) *Client {
	return &Client{engine: e}
}

func (c *Client) transcribeAudio(ctx *gin.Context, translate bool) {
	file, _, err := ctx.Request.FormFile("file")
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "file field is required"})
		return
	}
	defer file.Close()

	lang := strings.TrimSpace(ctx.PostForm("language"))
	respFmt := strings.TrimSpace(ctx.PostForm("response_format"))
	if respFmt == "" {
		respFmt = "json"
	}

	wordTS := false
	for _, g := range ctx.PostFormArray("timestamp_granularities[]") {
		if g == "word" {
			wordTS = true
			break
		}
	}

	if translate && !c.engine.IsMultilingual() {
		ctx.JSON(http.StatusUnprocessableEntity, gin.H{"error": "model does not support translation"})
		return
	}

	audioTmp, err := os.CreateTemp("", "omw-audio-*")
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	audioPath := audioTmp.Name()
	defer os.Remove(audioPath)

	if _, err = io.Copy(audioTmp, file); err != nil {
		audioTmp.Close()
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	audioTmp.Close()

	pcmTmp, err := os.CreateTemp("", "omw-pcm-*.f32le")
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	pcmPath := pcmTmp.Name()
	pcmTmp.Close()
	defer os.Remove(pcmPath)

	out, err := exec.Command("ffmpeg",
		"-loglevel", "error",
		"-i", audioPath,
		"-ar", "16000",
		"-ac", "1",
		"-f", "f32le",
		"-y", pcmPath,
	).CombinedOutput()
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "audio conversion failed: " + string(out)})
		return
	}

	pcmData, err := os.ReadFile(pcmPath)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	samples := bytesToFloat32(pcmData)

	var (
		text string
		segs []whisperlib.Segment
	)

	if translate {
		text, segs, err = c.engine.Translate(samples, wordTS)
	} else {
		text, segs, err = c.engine.Transcribe(samples, lang, wordTS)
	}
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	task := "transcribe"
	if translate {
		task = "translate"
	}

	fmtSegs := toFormatSegments(segs)
	duration := float64(len(samples)) / 16000.0

	switch respFmt {
	case "text":
		ctx.String(http.StatusOK, "%s", text)
	case "segment":
		ctx.JSON(http.StatusOK, format.SegmentResponse{Segments: fmtSegs})
	case "verbose_json":
		r := format.VerboseResponse{
			Task:     task,
			Language: lang,
			Duration: duration,
			Text:     text,
			Segments: fmtSegs,
		}
		if wordTS {
			r.Words = flatWords(segs)
		}
		ctx.JSON(http.StatusOK, r)
	case "model":
		ctx.JSON(http.StatusOK, format.ModelResponse{
			Task:     task,
			Language: lang,
			Duration: duration,
			Text:     text,
			Model: format.ModelInfo{
				ID:           c.engine.ModelID(),
				Object:       "model",
				Multilingual: c.engine.IsMultilingual(),
			},
			Segments: fmtSegs,
		})
	default:
		ctx.JSON(http.StatusOK, format.TranscriptionResponse{Text: text})
	}
}

func (c *Client) Transcribe(ctx *gin.Context) {
	c.transcribeAudio(ctx, false)
}

func (c *Client) Translate(ctx *gin.Context) {
	c.transcribeAudio(ctx, true)
}

func (c *Client) Models(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, format.ModelListResponse{
		Object: "list",
		Data: []format.ModelInfo{{
			ID:           c.engine.ModelID(),
			Object:       "model",
			Multilingual: c.engine.IsMultilingual(),
		}},
	})
}

func Serve(c *Client, port int, middleware ...gin.HandlerFunc) error {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	for _, m := range middleware {
		r.Use(m)
	}
	r.POST("/v1/audio/transcriptions", c.Transcribe)
	r.POST("/v1/audio/translations", c.Translate)
	r.GET("/v1/models", c.Models)
	return r.Run(fmt.Sprintf("0.0.0.0:%d", port))
}

func toFormatSegments(segs []whisperlib.Segment) []format.Segment {
	out := make([]format.Segment, len(segs))
	for i, s := range segs {
		var words []format.Word
		for _, w := range s.Words {
			words = append(words, format.Word{Word: w.Word, Start: w.Start, End: w.End})
		}
		out[i] = format.Segment{ID: s.ID, Start: s.Start, End: s.End, Text: s.Text, Words: words}
	}
	return out
}

func flatWords(segs []whisperlib.Segment) []format.Word {
	var out []format.Word
	for _, s := range segs {
		for _, w := range s.Words {
			out = append(out, format.Word{Word: w.Word, Start: w.Start, End: w.End})
		}
	}
	return out
}

func bytesToFloat32(data []byte) []float32 {
	n := len(data) / 4
	out := make([]float32, n)
	for i := range out {
		bits := binary.LittleEndian.Uint32(data[i*4:])
		out[i] = math.Float32frombits(bits)
	}
	return out
}
