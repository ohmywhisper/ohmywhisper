package model

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
	"ohmywhisper/config"
)

type ExternalEntry struct {
	Name      string `yaml:"name"`
	File      string `yaml:"file"`
	Source    string `yaml:"source"`
	Hub       string `yaml:"hub"`
	SizeBytes int64  `yaml:"size_bytes,omitempty"`
}

type hubSync struct {
	Hub    string          `yaml:"hub"`
	Source string          `yaml:"source"`
	Models []ExternalEntry `yaml:"models"`
}

func NormalizeHub(raw string) string {
	u := strings.TrimRight(raw, "/")
	for _, sfx := range []string{"/tree/main", "/blob/main", "/tree/master", "/blob/master"} {
		if strings.HasSuffix(u, sfx) {
			u = strings.TrimSuffix(u, sfx)
		}
	}
	if strings.Contains(u, "/resolve/") {
		return u
	}
	return u + "/resolve/main"
}

func SourceName(hubURL string) string {
	u := strings.TrimRight(hubURL, "/")
	u = strings.TrimSuffix(u, "/resolve/main")
	parts := strings.Split(u, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return hubURL
}

func hfOwnerRepo(hubURL string) (string, bool) {
	u := strings.TrimRight(hubURL, "/")
	u = strings.TrimSuffix(u, "/resolve/main")
	if !strings.HasPrefix(u, "https://huggingface.co/") {
		return "", false
	}
	rest := strings.TrimPrefix(u, "https://huggingface.co/")
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 2 || parts[1] == "" {
		return "", false
	}
	return parts[0] + "/" + parts[1], true
}

var genericFileStems = []string{
	"pytorch_model", "model", "model_weights", "flax_model", "tf_model",
}

func isGenericStem(stem string) bool {
	for _, g := range genericFileStems {
		if stem == g || strings.HasPrefix(stem, g+"-") {
			return true
		}
	}
	return false
}

func SyncHub(hubURL string) ([]ExternalEntry, error) {
	ownerRepo, ok := hfOwnerRepo(hubURL)
	if !ok {
		return nil, fmt.Errorf("not a supported HuggingFace URL: %s", hubURL)
	}
	apiURL := "https://huggingface.co/api/models/" + ownerRepo
	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("fetch hub info: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hub API HTTP %d for %s", resp.StatusCode, apiURL)
	}
	var info struct {
		Siblings []struct {
			Rfilename string `json:"rfilename"`
		} `json:"siblings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("parse hub info: %w", err)
	}
	source := SourceName(hubURL)
	seen := map[string]bool{}
	var entries []ExternalEntry
	for _, s := range info.Siblings {
		if !strings.HasSuffix(s.Rfilename, ".bin") {
			continue
		}
		stem := strings.TrimSuffix(s.Rfilename, ".bin")
		name := strings.TrimPrefix(stem, "ggml-")
		if isGenericStem(name) {
			name = source
		}
		if seen[name] {
			continue
		}
		seen[name] = true

		fileURL := strings.TrimRight(hubURL, "/") + "/" + s.Rfilename
		sz := headSize(fileURL)

		entries = append(entries, ExternalEntry{
			Name:      name,
			File:      s.Rfilename,
			Source:    source,
			Hub:       hubURL,
			SizeBytes: sz,
		})
	}
	return entries, nil
}

func headSize(url string) int64 {
	resp, err := http.Head(url)
	if err != nil {
		return 0
	}
	return resp.ContentLength
}

func externalCachePath(cfg *config.Config) string {
	return filepath.Join(filepath.Dir(cfg.ModelDir), "external_catalog.yml")
}

func LoadExternalCatalog(cfg *config.Config) []ExternalEntry {
	if cfg == nil {
		return nil
	}
	data, err := os.ReadFile(externalCachePath(cfg))
	if err != nil {
		return nil
	}
	var syncs []hubSync
	if err := yaml.Unmarshal(data, &syncs); err != nil {
		return nil
	}
	seen := map[string]bool{}
	var all []ExternalEntry
	for _, s := range syncs {
		for _, e := range s.Models {
			key := e.Hub + "\x00" + e.Name
			if seen[key] {
				continue
			}
			seen[key] = true
			all = append(all, e)
		}
	}
	return all
}

func saveExternalCatalog(cfg *config.Config, syncs []hubSync) error {
	path := externalCachePath(cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(syncs)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func SyncAllHubs(cfg *config.Config) (int, error) {
	var syncs []hubSync
	for _, raw := range cfg.ExtraHubs {
		hubURL := NormalizeHub(raw)
		source := SourceName(hubURL)
		entries, err := SyncHub(hubURL)
		if err != nil {
			return 0, fmt.Errorf("sync %s: %w", source, err)
		}
		syncs = append(syncs, hubSync{Hub: hubURL, Source: source, Models: entries})
	}
	if err := saveExternalCatalog(cfg, syncs); err != nil {
		return 0, err
	}
	total := 0
	for _, s := range syncs {
		total += len(s.Models)
	}
	return total, nil
}
