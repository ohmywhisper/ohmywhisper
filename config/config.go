package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	ModelDir   string   `yaml:"model_dir"`
	Hub        string   `yaml:"hub"`
	ExtraHubs  []string `yaml:"extra_hubs,omitempty"`
	ServerURL  string   `yaml:"server_url"`
	WhisperSrc string   `yaml:"whisper_src"`
	GPU        bool     `yaml:"gpu"`
	GPUDevice  int      `yaml:"gpu_device"`
}

func Default() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		ModelDir:  filepath.Join(home, ".ohmywhisper", "models"),
		Hub:       "https://huggingface.co/ggerganov/whisper.cpp/resolve/main",
		ServerURL: "http://localhost:3199",
		GPU:       true,
		GPUDevice: 0,
	}
}

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ohmywhisper", "config.yml"), nil
}

func Load() (*Config, error) {
	cfg := Default()
	path, err := configPath()
	if err != nil {
		return cfg, nil
	}
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

func (c *Config) Save() error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (c *Config) EnsureDir() error {
	return os.MkdirAll(c.ModelDir, 0755)
}
