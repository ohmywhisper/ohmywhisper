package model

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"ohmywhisper/config"
)

func Convert(srcPath, name string, cfg *config.Config) error {
	script := findConvertScript(cfg)
	if script == "" {
		return fmt.Errorf(
			"conversion script not found\n" +
				"set whisper_src in ~/.ohmywhisper/config.yml pointing to a whisper.cpp clone:\n" +
				"  git clone https://github.com/ggml-org/whisper.cpp\n" +
				"then set: whisper_src: /path/to/whisper.cpp",
		)
	}

	if err := cfg.EnsureDir(); err != nil {
		return err
	}

	if name == "" {
		base := filepath.Base(srcPath)
		name = strings.SplitN(base, ".", 2)[0]
	}
	outPath := filepath.Join(cfg.ModelDir, name+".bin")

	fmt.Printf("converting %s\n", srcPath)
	fmt.Printf("output     %s\n", outPath)

	args := []string{script, srcPath, outPath}
	if cfg.WhisperSrc != "" {
		args = []string{script, srcPath, cfg.WhisperSrc, outPath}
	}

	cmd := exec.Command("python3", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("conversion failed: %w", err)
	}

	fmt.Printf("saved %s\n", outPath)
	return nil
}

func findConvertScript(cfg *config.Config) string {
	var candidates []string
	if cfg.WhisperSrc != "" {
		candidates = append(candidates,
			filepath.Join(cfg.WhisperSrc, "models", "convert-pt-to-ggml.py"),
			filepath.Join(cfg.WhisperSrc, "models", "convert-whisper-to-ggml.py"),
		)
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	for _, name := range []string{"convert-pt-to-ggml.py", "convert-whisper-to-ggml.py"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}
