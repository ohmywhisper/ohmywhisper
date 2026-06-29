package model

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ohmywhisper/config"
)

const chunkThreshold = 50 * 1024 * 1024
const numChunks = 4

func Pull(name string, cfg *config.Config) error {
	return doPull(name, cfg, nil)
}

func PullWithProgress(name string, cfg *config.Config, cb func(label string, written, total int64)) error {
	return doPull(name, cfg, cb)
}

func doPull(name string, cfg *config.Config, cb func(string, int64, int64)) error {
	if err := cfg.EnsureDir(); err != nil {
		return err
	}

	if strings.HasPrefix(name, "http://") || strings.HasPrefix(name, "https://") {
		filename := filepath.Base(strings.SplitN(name, "?", 2)[0])
		destPath := filepath.Join(cfg.ModelDir, filename)
		if _, err := os.Stat(destPath); err == nil {
			if cb == nil {
				modelName := strings.TrimPrefix(strings.TrimSuffix(filename, ".bin"), "ggml-")
				fmt.Printf("model %s already downloaded\n", modelName)
			}
			return nil
		}
		if cb == nil {
			fmt.Println("pulling manifest")
		}
		return download(name, filename, destPath, cb)
	}

	for _, ext := range LoadExternalCatalog(cfg) {
		if ext.Name == name || ext.File == name {
			destPath := filepath.Join(cfg.ModelDir, ext.File)
			if _, err := os.Stat(destPath); err == nil {
				if cb == nil {
					fmt.Printf("model %s already downloaded\n", ext.Name)
				}
				return nil
			}
			url := strings.TrimRight(ext.Hub, "/") + "/" + ext.File
			if cb == nil {
				fmt.Println("pulling manifest")
			}
			return download(url, ext.File, destPath, cb)
		}
	}

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

	destPath := filepath.Join(cfg.ModelDir, entry.File)
	if _, err := os.Stat(destPath); err == nil {
		if cb == nil {
			fmt.Printf("model %s already downloaded\n", entry.Name)
		}
		return nil
	}

	hubs := append([]string{cfg.Hub}, cfg.ExtraHubs...)
	if cb == nil {
		fmt.Println("pulling manifest")
	}

	var lastErr error
	for _, hub := range hubs {
		normalized := NormalizeHub(hub)
		url := normalized + "/" + entry.File
		resp, err := http.Head(url)
		if err != nil || resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("not available at %s", hub)
			continue
		}
		lastErr = download(url, entry.File, destPath, cb)
		if lastErr == nil {
			return nil
		}
	}
	return lastErr
}

func download(url, label, destPath string, cb func(string, int64, int64)) error {
	head, err := http.Head(url)
	if err != nil {
		return fmt.Errorf("head: %w", err)
	}
	total := head.ContentLength
	if head.Header.Get("Accept-Ranges") == "bytes" && total > chunkThreshold {
		return downloadChunked(url, label, destPath, total, cb)
	}
	return downloadSingle(url, label, destPath, total, cb)
}

func downloadSingle(url, label, destPath string, total int64, cb func(string, int64, int64)) error {
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

	var written atomic.Int64
	var done atomic.Int32

	go func() {
		for done.Load() == 0 {
			w := written.Load()
			if cb != nil {
				cb(label, w, total)
			} else {
				flushProgress(label, w, total)
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()

	buf := make([]byte, 32*1024)
	var copyErr error
	for {
		n, rErr := resp.Body.Read(buf)
		if n > 0 {
			if _, wErr := f.Write(buf[:n]); wErr != nil {
				copyErr = wErr
				break
			}
			written.Add(int64(n))
		}
		if rErr == io.EOF {
			break
		}
		if rErr != nil {
			copyErr = rErr
			break
		}
	}
	f.Close()
	done.Store(1)
	time.Sleep(80 * time.Millisecond)

	if copyErr != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("download: %w", copyErr)
	}

	if cb == nil {
		flushProgress(label, written.Load(), total)
		fmt.Println()
	}

	return os.Rename(tmpPath, destPath)
}

func downloadChunked(url, label, destPath string, total int64, cb func(string, int64, int64)) error {
	chunkSize := total / numChunks
	tmpPaths := make([]string, numChunks)

	var totalWritten atomic.Int64
	var done atomic.Int32

	go func() {
		for done.Load() == 0 {
			w := totalWritten.Load()
			if cb != nil {
				cb(label, w, total)
			} else {
				flushProgress(label, w, total)
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()

	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	for i := 0; i < numChunks; i++ {
		start := int64(i) * chunkSize
		end := start + chunkSize - 1
		if i == numChunks-1 {
			end = total - 1
		}
		tmpPaths[i] = fmt.Sprintf("%s.chunk.%d", destPath, i)

		wg.Add(1)
		go func(s, e int64, tmp string) {
			defer wg.Done()
			if err := downloadChunk(url, tmp, s, e, &totalWritten); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}(start, end, tmpPaths[i])
	}

	wg.Wait()
	done.Store(1)
	time.Sleep(80 * time.Millisecond)

	if firstErr != nil {
		for _, p := range tmpPaths {
			os.Remove(p)
		}
		return firstErr
	}

	if cb == nil {
		flushProgress(label, total, total)
		fmt.Println()
	}

	tmpFinal := destPath + ".tmp"
	f, err := os.Create(tmpFinal)
	if err != nil {
		return err
	}

	for _, p := range tmpPaths {
		chunk, err := os.Open(p)
		if err != nil {
			f.Close()
			return err
		}
		_, err = io.Copy(f, chunk)
		chunk.Close()
		os.Remove(p)
		if err != nil {
			f.Close()
			return err
		}
	}
	f.Close()

	return os.Rename(tmpFinal, destPath)
}

func downloadChunk(url, tmpPath string, start, end int64, written *atomic.Int64) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("chunk: HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	buf := make([]byte, 32*1024)
	for {
		n, rErr := resp.Body.Read(buf)
		if n > 0 {
			if _, wErr := f.Write(buf[:n]); wErr != nil {
				f.Close()
				return wErr
			}
			written.Add(int64(n))
		}
		if rErr == io.EOF {
			break
		}
		if rErr != nil {
			f.Close()
			return rErr
		}
	}
	return f.Close()
}

func flushProgress(label string, written, total int64) {
	const width = 28
	if total > 0 {
		pct := float64(written) / float64(total)
		filled := int(pct * width)
		if filled > width {
			filled = width
		}
		bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
		fmt.Printf("\rpulling %-30s [%s] %3.0f%% %-10s",
			label, bar, pct*100, HumanSize(written))
	} else {
		fmt.Printf("\rpulling %-30s %s", label, HumanSize(written))
	}
}
