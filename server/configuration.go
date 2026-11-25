package main

import (
	"fmt"
	"strconv"
	"strings"
)

// configuration повторяет ключи из settings_schema в plugin.json посимвольно:
// Go сопоставляет ключи с json-тегами без учёта регистра, но подчёркивания
// значимы, поэтому теги обязаны совпадать с ключами консоли.
type configuration struct {
	DefaultAnonymous     bool   `json:"DefaultAnonymous"`
	HideResultsByDefault bool   `json:"HideResultsByDefault"`
	MaxOptions           string `json:"MaxOptions"`
	AnyoneCanEnd         bool   `json:"AnyoneCanEnd"`
	AdminsCanEnd         bool   `json:"AdminsCanEnd"`
	VotersNameFormat     string `json:"VotersNameFormat"`
	ShowVotersButton     bool   `json:"ShowVotersButton"`
}

const (
	defaultMaxOptions = 12
	hardMaxOptions    = 20
)

func (c *configuration) maxOptions() int {
	n, err := strconv.Atoi(strings.TrimSpace(c.MaxOptions))
	if err != nil || n < 2 {
		return defaultMaxOptions
	}
	if n > hardMaxOptions {
		return hardMaxOptions
	}
	return n
}

func (p *Plugin) OnConfigurationChange() error {
	var cfg configuration
	if err := p.API.LoadPluginConfiguration(&cfg); err != nil {
		return fmt.Errorf("не удалось прочитать настройки плагина: %w", err)
	}

	p.configurationLock.Lock()
	p.configuration = &cfg
	p.configurationLock.Unlock()

	return nil
}

func (p *Plugin) getConfiguration() *configuration {
	p.configurationLock.RLock()
	defer p.configurationLock.RUnlock()

	if p.configuration == nil {
		return &configuration{MaxOptions: strconv.Itoa(defaultMaxOptions), ShowVotersButton: true}
	}
	return p.configuration
}
