package whisper

/*
#cgo CFLAGS: -I${SRCDIR}/../../lib/include
#include "whisper.h"
#include <stdlib.h>
*/
import "C"
import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"
)

const (
	sampleRate   = 16000
	chunkSamples = sampleRate * 30
)

type Word struct {
	Word  string
	Start float64
	End   float64
}

type Segment struct {
	ID    int
	Start float64
	End   float64
	Text  string
	Words []Word
}

type Engine struct {
	ctx          *C.struct_whisper_context
	modelPath    string
	multilingual bool
	mu           sync.Mutex
}

func NewEngine(modelPath string, gpu bool, gpuDevice int) (*Engine, error) {
	cPath := C.CString(modelPath)
	defer C.free(unsafe.Pointer(cPath))

	cparams := C.whisper_context_default_params()
	cparams.use_gpu = C.bool(gpu)
	cparams.gpu_device = C.int(gpuDevice)

	ctx := C.whisper_init_from_file_with_params(cPath, cparams)
	if ctx == nil {
		return nil, fmt.Errorf("whisper: failed to load model %s", modelPath)
	}

	return &Engine{
		ctx:          ctx,
		modelPath:    modelPath,
		multilingual: C.whisper_is_multilingual(ctx) != 0,
	}, nil
}

func (e *Engine) Close() {
	C.whisper_free(e.ctx)
}

func (e *Engine) IsMultilingual() bool {
	return e.multilingual
}

func (e *Engine) ModelID() string {
	base := filepath.Base(e.modelPath)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func (e *Engine) Transcribe(samples []float32, lang string, wordTS bool) (string, []Segment, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.run(samples, lang, false, wordTS)
}

func (e *Engine) Translate(samples []float32, wordTS bool) (string, []Segment, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.run(samples, "", true, wordTS)
}

func (e *Engine) TranscribeStream(samples []float32, lang string, wordTS bool, cb func(Segment)) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.runStream(samples, lang, false, wordTS, cb)
}

func (e *Engine) TranslateStream(samples []float32, wordTS bool, cb func(Segment)) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.runStream(samples, "", true, wordTS, cb)
}

func (e *Engine) runStream(samples []float32, lang string, translate bool, wordTS bool, cb func(Segment)) error {
	if len(samples) == 0 {
		return nil
	}
	var timeOff float64
	segID := 0
	for i := 0; i < len(samples); i += chunkSamples {
		end := i + chunkSamples
		if end > len(samples) {
			end = len(samples)
		}
		chunk := samples[i:end]
		_, segs, err := e.process(chunk, lang, translate, wordTS, timeOff, &segID)
		if err != nil {
			return err
		}
		for _, seg := range segs {
			cb(seg)
		}
		timeOff += float64(len(chunk)) / sampleRate
	}
	return nil
}

func (e *Engine) run(samples []float32, lang string, translate bool, wordTS bool) (string, []Segment, error) {
	if len(samples) == 0 {
		return "", nil, nil
	}

	var parts []string
	var all []Segment
	var timeOff float64
	segID := 0

	for i := 0; i < len(samples); i += chunkSamples {
		end := i + chunkSamples
		if end > len(samples) {
			end = len(samples)
		}
		chunk := samples[i:end]

		text, segs, err := e.process(chunk, lang, translate, wordTS, timeOff, &segID)
		if err != nil {
			return "", nil, err
		}

		parts = append(parts, text)
		all = append(all, segs...)
		timeOff += float64(len(chunk)) / sampleRate
	}

	return strings.TrimSpace(strings.Join(parts, "")), all, nil
}

func (e *Engine) process(samples []float32, lang string, translate bool, wordTS bool, timeOff float64, segID *int) (string, []Segment, error) {
	params := C.whisper_full_default_params(C.WHISPER_SAMPLING_BEAM_SEARCH)
	params.print_progress = C.bool(false)
	params.print_realtime = C.bool(false)
	params.print_special = C.bool(false)
	params.print_timestamps = C.bool(false)
	params.no_timestamps = C.bool(false)
	params.single_segment = C.bool(false)
	params.no_speech_thold = C.float(0.3)
	params.translate = C.bool(translate)
	params.token_timestamps = C.bool(wordTS)
	if wordTS {
		params.thold_pt = C.float(0.01)
	}

	if lang != "" && lang != "auto" {
		cLang := C.CString(lang)
		defer C.free(unsafe.Pointer(cLang))
		params.language = cLang
		params.detect_language = C.bool(false)
	}

	ret := C.whisper_full(e.ctx, params, (*C.float)(unsafe.Pointer(&samples[0])), C.int(len(samples)))
	if ret != 0 {
		return "", nil, fmt.Errorf("whisper_full returned %d", ret)
	}

	n := int(C.whisper_full_n_segments(e.ctx))
	var sb strings.Builder
	segs := make([]Segment, 0, n)

	eot := C.whisper_token_eot(e.ctx)

	for i := 0; i < n; i++ {
		text := C.GoString(C.whisper_full_get_segment_text(e.ctx, C.int(i)))
		t0 := float64(C.whisper_full_get_segment_t0(e.ctx, C.int(i)))*0.01 + timeOff
		t1 := float64(C.whisper_full_get_segment_t1(e.ctx, C.int(i)))*0.01 + timeOff
		sb.WriteString(text)

		var words []Word
		if wordTS {
			nt := int(C.whisper_full_n_tokens(e.ctx, C.int(i)))
			for j := 0; j < nt; j++ {
				td := C.whisper_full_get_token_data(e.ctx, C.int(i), C.int(j))
				if td.id >= eot || td.t0 < 0 {
					continue
				}
				wtext := C.GoString(C.whisper_full_get_token_text(e.ctx, C.int(i), C.int(j)))
				words = append(words, Word{
					Word:  wtext,
					Start: float64(td.t0)*0.01 + timeOff,
					End:   float64(td.t1)*0.01 + timeOff,
				})
			}
		}

		segs = append(segs, Segment{ID: *segID, Start: t0, End: t1, Text: text, Words: words})
		*segID++
	}

	return sb.String(), segs, nil
}
