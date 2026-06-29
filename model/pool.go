package model

import (
	"fmt"
	"sync"
	"time"

	whisperlib "ohmywhisper/api/whisper"
	"ohmywhisper/config"
)

type LoadedModel struct {
	Name   string    `json:"name"`
	Path   string    `json:"path"`
	Since  time.Time `json:"since"`
	engine *whisperlib.Engine
}

func (m *LoadedModel) Engine() *whisperlib.Engine {
	return m.engine
}

type Pool struct {
	mu     sync.RWMutex
	models map[string]*LoadedModel
	cfg    *config.Config
}

func NewPool(cfg *config.Config) *Pool {
	return &Pool{
		models: make(map[string]*LoadedModel),
		cfg:    cfg,
	}
}

func (p *Pool) LoadPath(name, path string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.models[name]; ok {
		return nil
	}
	e, err := whisperlib.NewEngine(path, p.cfg.GPU, p.cfg.GPUDevice)
	if err != nil {
		return err
	}
	p.models[name] = &LoadedModel{Name: name, Path: path, Since: time.Now(), engine: e}
	return nil
}

func (p *Pool) Load(name string) error {
	p.mu.Lock()
	if _, ok := p.models[name]; ok {
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()

	path, err := ResolvePath(name, p.cfg)
	if err != nil {
		return err
	}
	return p.LoadPath(name, path)
}

func (p *Pool) Unload(name string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	m, ok := p.models[name]
	if !ok {
		return fmt.Errorf("model %q is not loaded", name)
	}
	m.engine.Close()
	delete(p.models, name)
	return nil
}

func (p *Pool) Get(name string) (*LoadedModel, error) {
	p.mu.RLock()
	m, ok := p.models[name]
	p.mu.RUnlock()
	if ok {
		return m, nil
	}
	if err := p.Load(name); err != nil {
		return nil, err
	}
	p.mu.RLock()
	m = p.models[name]
	p.mu.RUnlock()
	return m, nil
}

func (p *Pool) Default() (*LoadedModel, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, m := range p.models {
		return m, nil
	}
	return nil, fmt.Errorf("no models loaded — use 'ohmywhisper start <model>' or 'ohmywhisper serve --model <name>'")
}

func (p *Pool) List() []LoadedModel {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]LoadedModel, 0, len(p.models))
	for _, m := range p.models {
		out = append(out, *m)
	}
	return out
}

func (p *Pool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, m := range p.models {
		m.engine.Close()
	}
	p.models = make(map[string]*LoadedModel)
}
