package model

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ohmywhisper/config"
)

type Modelfile struct {
	From     string
	Language string
}

func ParseModelfile(path string) (*Modelfile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	mf := &Modelfile{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}
		switch strings.ToUpper(parts[0]) {
		case "FROM":
			mf.From = strings.TrimSpace(parts[1])
		case "LANGUAGE":
			mf.Language = strings.TrimSpace(parts[1])
		}
	}
	if mf.From == "" {
		return nil, fmt.Errorf("Modelfile missing FROM instruction")
	}
	return mf, sc.Err()
}

func CreateModel(name, modelfilePath string, cfg *config.Config) error {
	mf, err := ParseModelfile(modelfilePath)
	if err != nil {
		return err
	}

	if err := cfg.EnsureDir(); err != nil {
		return err
	}

	srcPath, err := ResolvePath(mf.From, cfg)
	if err != nil {
		return fmt.Errorf("FROM %s: %w", mf.From, err)
	}

	destBin := filepath.Join(cfg.ModelDir, name+".bin")
	_ = os.Remove(destBin)
	if err := os.Symlink(srcPath, destBin); err != nil {
		return err
	}

	data, err := os.ReadFile(modelfilePath)
	if err != nil {
		return err
	}
	destMf := filepath.Join(cfg.ModelDir, name+".modelfile")
	if err := os.WriteFile(destMf, data, 0644); err != nil {
		return err
	}

	fmt.Printf("created %s from %s\n", name, mf.From)
	return nil
}
