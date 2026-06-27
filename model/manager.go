package model

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ohmywhisper/config"
)

type LocalModel struct {
	Name    string
	Path    string
	Size    int64
	ModTime time.Time
}

func List(cfg *config.Config) ([]LocalModel, error) {
	entries, err := os.ReadDir(cfg.ModelDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []LocalModel
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".bin") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".bin")
		name = strings.TrimPrefix(name, "ggml-")
		out = append(out, LocalModel{
			Name:    name,
			Path:    filepath.Join(cfg.ModelDir, e.Name()),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}
	return out, nil
}

func Remove(name string, cfg *config.Config) error {
	path, err := ResolvePath(name, cfg)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	mfPath := strings.TrimSuffix(path, ".bin") + ".modelfile"
	_ = os.Remove(mfPath)
	return nil
}

func ResolvePath(name string, cfg *config.Config) (string, error) {
	if filepath.IsAbs(name) {
		if _, err := os.Stat(name); err == nil {
			return name, nil
		}
	}
	candidates := []string{
		filepath.Join(cfg.ModelDir, name),
		filepath.Join(cfg.ModelDir, name+".bin"),
		filepath.Join(cfg.ModelDir, "ggml-"+name+".bin"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("model %q not found (run 'ohmywhisper pull %s')", name, name)
}

func Show(name string, cfg *config.Config) (*LocalModel, error) {
	path, err := ResolvePath(name, cfg)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	base := strings.TrimSuffix(filepath.Base(path), ".bin")
	base = strings.TrimPrefix(base, "ggml-")
	return &LocalModel{
		Name:    base,
		Path:    path,
		Size:    info.Size(),
		ModTime: info.ModTime(),
	}, nil
}

func HumanSize(n int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case n >= GB:
		return fmt.Sprintf("%.2f GB", float64(n)/GB)
	case n >= MB:
		return fmt.Sprintf("%.1f MB", float64(n)/MB)
	case n >= KB:
		return fmt.Sprintf("%.1f KB", float64(n)/KB)
	default:
		return fmt.Sprintf("%d B", n)
	}
}
