//go:build linux && amd64

package whisper

/*
#cgo LDFLAGS: -L${SRCDIR}/../../lib -Wl,--start-group -lwhisper -lggml -lggml-cpu -lggml-base -Wl,--end-group -lstdc++ -lgomp -lm -lpthread
*/
import "C"
