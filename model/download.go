package model

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"ohmywhisper/config"
)

func Pull(name string, cfg *config.Config) error {
	entry := FindByName(name)
	if entry == nil {
		if strings.HasSuffix(name, ".bin") {
			trimmed := strings.TrimPrefix(name, "ggml-")
			trimmed = strings.TrimSuffix(trimmed, ".bin")
			entry = &CatalogEntry{Name: trimmed, File: name}
		} else {
			return fmt.Errorf("model %q not found in catalog, run 'ohmywhisper search %s'", name, name)
		}
	}

	if err := cfg.EnsureDir(); err != nil {
		return err
	}

	destPath := filepath.Join(cfg.ModelDir, entry.File)
	if _, err := os.Stat(destPath); err == nil {
		fmt.Printf("model %s already downloaded\n", entry.Name)
		return nil
	}

	url := strings.TrimRight(cfg.Hub, "/") + "/" + entry.File
	fmt.Println("pulling manifest")

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download: HTTP %d for %s", resp.StatusCode, url)
	}

	tmpPath := destPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	pr := &progressReader{
		r:     resp.Body,
		total: resp.ContentLength,
		label: entry.File,
	}
	go pr.tick()

	_, copyErr := io.Copy(f, pr)
	f.Close()
	pr.done.Store(1)
	time.Sleep(80 * time.Millisecond)

	if copyErr != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("download: %w", copyErr)
	}

	pr.flush()
	fmt.Println()

	return os.Rename(tmpPath, destPath)
}

type progressReader struct {
	r       io.Reader
	total   int64
	label   string
	written atomic.Int64
	done    atomic.Int32
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.written.Add(int64(n))
	return n, err
}

func (p *progressReader) tick() {
	for p.done.Load() == 0 {
		p.flush()
		time.Sleep(100 * time.Millisecond)
	}
}

func (p *progressReader) flush() {
	const width = 28
	w := p.written.Load()
	if p.total > 0 {
		pct := float64(w) / float64(p.total)
		filled := int(pct * width)
		if filled > width {
			filled = width
		}
		bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
		fmt.Printf("\rpulling %-30s [%s] %3.0f%% %-10s",
			p.label, bar, pct*100, HumanSize(w))
	} else {
		fmt.Printf("\rpulling %-30s %s", p.label, HumanSize(w))
	}
}
