//go:build linux && (arm || arm64)

package whisper

/*
#cgo LDFLAGS: -L${SRCDIR}/../../lib -Wl,--start-group -lwhisper -lggml -lggml-cpu -lggml-base -Wl,--end-group -lstdc++ -lm -lpthread
*/
import "C"
