package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	ModelDir   string `yaml:"model_dir"`
	Hub        string `yaml:"hub"`
	ServerURL  string `yaml:"server_url"`
	WhisperSrc string `yaml:"whisper_src"`
}

func Default() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		ModelDir:  filepath.Join(home, ".ohmywhisper", "models"),
		Hub:       "https://huggingface.co/ggerganov/whisper.cpp/resolve/main",
		ServerURL: "http://localhost:3199",
	}
}

func Load() (*Config, error) {
	cfg := Default()
	home, err := os.UserHomeDir()
	if err != nil {
		return cfg, nil
	}
	path := filepath.Join(home, ".ohmywhisper", "config.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	if cfg.ModelDir == "" {
		cfg.ModelDir = Default().ModelDir
	}
	if cfg.Hub == "" {
		cfg.Hub = Default().Hub
	}
	if cfg.ServerURL == "" {
		cfg.ServerURL = Default().ServerURL
	}
	return cfg, nil
}

func (c *Config) EnsureDir() error {
	return os.MkdirAll(c.ModelDir, 0755)
}
