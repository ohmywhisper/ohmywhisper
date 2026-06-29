package api

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	whisperlib "ohmywhisper/api/whisper"
	"ohmywhisper/config"
	"ohmywhisper/format"
	"ohmywhisper/model"
	"ohmywhisper/sysinfo"
)

type Client struct {
	pool        *model.Pool
	cfg         *config.Config
	reqTotal    atomic.Int64
	audioMillis atomic.Int64
	procMillis  atomic.Int64
}

func NewClient(pool *model.Pool, cfg *config.Config) *Client {
	return &Client{pool: pool, cfg: cfg}
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

	modelName := strings.TrimSpace(ctx.PostForm("model"))
	var lm *model.LoadedModel
	if modelName != "" {
		lm, err = c.pool.Get(modelName)
	} else {
		lm, err = c.pool.Default()
	}
	if err != nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	engine := lm.Engine()

	if translate && !engine.IsMultilingual() {
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
	audioDurMs := int64(float64(len(samples)) / 16000.0 * 1000)

	if ctx.PostForm("stream") == "true" {
		c.streamSegments(ctx, translate, engine, samples, lang, wordTS, audioDurMs)
		return
	}

	var (
		text string
		segs []whisperlib.Segment
	)

	t0 := time.Now()
	if translate {
		text, segs, err = engine.Translate(samples, wordTS)
	} else {
		text, segs, err = engine.Transcribe(samples, lang, wordTS)
	}
	c.audioMillis.Add(audioDurMs)
	c.procMillis.Add(time.Since(t0).Milliseconds())

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
				ID:           engine.ModelID(),
				Object:       "model",
				Multilingual: engine.IsMultilingual(),
			},
			Segments: fmtSegs,
		})
	default:
		ctx.JSON(http.StatusOK, format.TranscriptionResponse{Text: text})
	}
}

func (c *Client) streamSegments(ctx *gin.Context, translate bool, engine *whisperlib.Engine, samples []float32, lang string, wordTS bool, audioDurMs int64) {
	ctx.Header("Content-Type", "text/event-stream")
	ctx.Header("Cache-Control", "no-cache")
	ctx.Header("Connection", "keep-alive")
	ctx.Header("X-Accel-Buffering", "no")
	ctx.Writer.WriteHeader(http.StatusOK)

	var full strings.Builder

	send := func(v any) {
		b, _ := json.Marshal(v)
		fmt.Fprintf(ctx.Writer, "data: %s\n\n", b)
		ctx.Writer.Flush()
	}

	cb := func(seg whisperlib.Segment) {
		full.WriteString(seg.Text)
		ev := map[string]any{
			"type":  "segment",
			"id":    seg.ID,
			"start": seg.Start,
			"end":   seg.End,
			"text":  seg.Text,
		}
		if wordTS && len(seg.Words) > 0 {
			words := make([]map[string]any, len(seg.Words))
			for i, w := range seg.Words {
				words[i] = map[string]any{"word": w.Word, "start": w.Start, "end": w.End}
			}
			ev["words"] = words
		}
		send(ev)
	}

	t0 := time.Now()
	var err error
	if translate {
		err = engine.TranslateStream(samples, wordTS, cb)
	} else {
		err = engine.TranscribeStream(samples, lang, wordTS, cb)
	}
	c.audioMillis.Add(audioDurMs)
	c.procMillis.Add(time.Since(t0).Milliseconds())

	if err != nil {
		send(map[string]any{"type": "error", "error": err.Error()})
		return
	}

	duration := float64(len(samples)) / 16000.0
	send(map[string]any{
		"type":     "done",
		"text":     strings.TrimSpace(full.String()),
		"language": lang,
		"duration": duration,
	})
}

func (c *Client) Transcribe(ctx *gin.Context) {
	c.transcribeAudio(ctx, false)
}

func (c *Client) Translate(ctx *gin.Context) {
	c.transcribeAudio(ctx, true)
}

func (c *Client) Models(ctx *gin.Context) {
	loaded := c.pool.List()
	data := make([]format.ModelInfo, 0, len(loaded))
	for _, m := range loaded {
		e := m.Engine()
		if e == nil {
			continue
		}
		data = append(data, format.ModelInfo{
			ID:           m.Name,
			Object:       "model",
			Multilingual: e.IsMultilingual(),
		})
	}
	ctx.JSON(http.StatusOK, format.ModelListResponse{Object: "list", Data: data})
}

func (c *Client) Tags(ctx *gin.Context) {
	models, err := model.List(c.cfg)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	type tagModel struct {
		Name       string `json:"name"`
		Size       int64  `json:"size"`
		ModifiedAt string `json:"modified_at"`
	}
	tags := make([]tagModel, len(models))
	for i, m := range models {
		tags[i] = tagModel{
			Name:       m.Name,
			Size:       m.Size,
			ModifiedAt: m.ModTime.Format(time.RFC3339),
		}
	}
	ctx.JSON(http.StatusOK, gin.H{"models": tags})
}

func (c *Client) PullModel(ctx *gin.Context) {
	var req struct {
		Name string `json:"name"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Name == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "name required"})
		return
	}

	ctx.Header("Content-Type", "text/event-stream")
	ctx.Header("Cache-Control", "no-cache")
	ctx.Header("Connection", "keep-alive")
	ctx.Header("X-Accel-Buffering", "no")
	ctx.Writer.WriteHeader(http.StatusOK)

	send := func(v any) {
		b, _ := json.Marshal(v)
		fmt.Fprintf(ctx.Writer, "data: %s\n\n", b)
		ctx.Writer.Flush()
	}

	err := model.PullWithProgress(req.Name, c.cfg, func(label string, written, total int64) {
		send(map[string]any{
			"type":    "progress",
			"label":   label,
			"written": written,
			"total":   total,
		})
	})
	if err != nil {
		send(map[string]any{"type": "error", "error": err.Error()})
		return
	}
	send(map[string]any{"type": "done", "name": req.Name})
}

func (c *Client) Metrics(ctx *gin.Context) {
	var sb strings.Builder

	gauge := func(name, help string, val float64, label ...string) {
		fmt.Fprintf(&sb, "# HELP %s %s\n# TYPE %s gauge\n", name, help, name)
		if len(label) > 0 {
			fmt.Fprintf(&sb, "%s{%s} %g\n", name, strings.Join(label, ","), val)
		} else {
			fmt.Fprintf(&sb, "%s %g\n", name, val)
		}
	}

	counter := func(name, help string, val float64) {
		fmt.Fprintf(&sb, "# HELP %s %s\n# TYPE %s counter\n%s %g\n", name, help, name, name, val)
	}

	allModels, _ := model.List(c.cfg)
	var modelsDiskBytes int64
	for _, m := range allModels {
		modelsDiskBytes += m.Size
	}
	gauge("ohmywhisper_models_total", "Total downloaded models", float64(len(allModels)))
	gauge("ohmywhisper_models_disk_bytes", "Total size of downloaded models in bytes", float64(modelsDiskBytes))
	gauge("ohmywhisper_models_loaded", "Currently loaded models", float64(len(c.pool.List())))

	stats := sysinfo.Collect()
	gauge("ohmywhisper_rss_bytes", "Process RAM usage in bytes", float64(stats.RSSMB*1024*1024))
	gauge("ohmywhisper_cpu_usage_percent", "Process CPU usage percent", stats.CPUPct)
	gauge("ohmywhisper_cpu_threads", "CPU thread count", float64(sysinfo.CPUThreads()))

	fmt.Fprintf(&sb, "# HELP ohmywhisper_cpu_temp_celsius CPU temperature in Celsius\n# TYPE ohmywhisper_cpu_temp_celsius gauge\n")
	for zone, temp := range sysinfo.CPUTempCelsius() {
		fmt.Fprintf(&sb, "ohmywhisper_cpu_temp_celsius{zone=%q} %g\n", zone, temp)
	}

	gpuCount := 0
	if stats.GPU != nil {
		gpuCount = 1
		gauge("ohmywhisper_gpu_vram_used_bytes", "GPU VRAM used in bytes",
			float64(stats.GPU.VRAMMB*1024*1024),
			fmt.Sprintf("name=%q", stats.GPU.Name))
		gauge("ohmywhisper_gpu_usage_percent", "GPU utilization percent",
			stats.GPU.Pct,
			fmt.Sprintf("name=%q", stats.GPU.Name))
	}
	gauge("ohmywhisper_gpu_count", "Number of GPUs", float64(gpuCount))

	rxPS, txPS := sysinfo.NetworkSpeed()
	gauge("ohmywhisper_network_rx_bytes_per_sec", "Network receive bytes per second", float64(rxPS))
	gauge("ohmywhisper_network_tx_bytes_per_sec", "Network transmit bytes per second", float64(txPS))

	diskUsed, diskFree := sysinfo.DiskStats(c.cfg.ModelDir)
	gauge("ohmywhisper_disk_used_bytes", "Disk space used in bytes", float64(diskUsed))
	gauge("ohmywhisper_disk_free_bytes", "Disk space free in bytes", float64(diskFree))

	counter("ohmywhisper_requests_total", "Total API requests received", float64(c.reqTotal.Load()))

	audioMs := c.audioMillis.Load()
	procMs := c.procMillis.Load()
	rtf := 0.0
	if procMs > 0 {
		rtf = float64(audioMs) / float64(procMs)
	}
	gauge("ohmywhisper_processing_speed_rtf", "Audio processing speed ratio (audio secs / proc secs)", rtf)

	ctx.Header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	ctx.String(http.StatusOK, "%s", sb.String())
}

type psEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Since string `json:"since"`
}

type psResponse struct {
	Models []psEntry        `json:"models"`
	RSSMB  int64            `json:"rss_mb"`
	CPUPct float64          `json:"cpu_pct"`
	GPU    *sysinfo.GPUInfo `json:"gpu,omitempty"`
}

func (c *Client) PS(ctx *gin.Context) {
	loaded := c.pool.List()
	entries := make([]psEntry, len(loaded))
	for i, m := range loaded {
		entries[i] = psEntry{Name: m.Name, Path: m.Path, Since: m.Since.Format("2006-01-02 15:04:05")}
	}
	stats := sysinfo.Collect()
	ctx.JSON(http.StatusOK, psResponse{
		Models: entries,
		RSSMB:  stats.RSSMB,
		CPUPct: stats.CPUPct,
		GPU:    stats.GPU,
	})
}

func (c *Client) Load(ctx *gin.Context) {
	var req struct {
		Name string `json:"name"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Name == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "name required"})
		return
	}
	if err := c.pool.Load(req.Name); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"status": "loaded", "name": req.Name})
}

func (c *Client) Unload(ctx *gin.Context) {
	name := ctx.Param("name")
	if err := c.pool.Unload(name); err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"status": "unloaded", "name": name})
}

func Serve(c *Client, port int, middleware ...gin.HandlerFunc) error {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(func(ctx *gin.Context) {
		c.reqTotal.Add(1)
		ctx.Next()
	})
	for _, m := range middleware {
		r.Use(m)
	}
	r.POST("/v1/audio/transcriptions", c.Transcribe)
	r.POST("/v1/audio/translations", c.Translate)
	r.GET("/v1/models", c.Models)
	r.GET("/api/ps", c.PS)
	r.POST("/api/load", c.Load)
	r.DELETE("/api/unload/:name", c.Unload)
	r.POST("/api/pull", c.PullModel)
	r.GET("/api/tags", c.Tags)
	r.GET("/metrics", c.Metrics)
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
