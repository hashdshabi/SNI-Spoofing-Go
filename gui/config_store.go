package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"sni-spoofing-go/guiapi"
)

type Preset struct {
	FakeSNI  string `json:"fakeSni"`
	Upstream string `json:"upstream"`
}

type SavedAppConfig struct {
	Config  ProxyConfig `json:"config"`
	Presets []Preset    `json:"presets"`
}

func configINIPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(exe), "config.ini"), nil
}

func defaultPresets() []Preset {
	return []Preset{
		{FakeSNI: "developers.cloudflare.com", Upstream: "104.16.2.189"},
		{FakeSNI: "developers.cloudflare.com", Upstream: "104.16.6.189"},
	}
}

func LoadSavedConfig(a *App) (SavedAppConfig, error) {
	cfg := a.GetDefaultConfig()
	presets := defaultPresets()
	path, err := configINIPath()
	if err != nil {
		return SavedAppConfig{Config: cfg, Presets: presets}, err
	}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return SavedAppConfig{Config: cfg, Presets: presets}, nil
	}
	if err != nil {
		return SavedAppConfig{Config: cfg, Presets: presets}, err
	}
	defer f.Close()

	section := ""
	loadedPresets := make([]Preset, 0)
	pending := map[int]*Preset{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if section == "presets" && strings.HasPrefix(key, "preset.") {
			p, ok := strings.CutPrefix(key, "preset.")
			idxText, field, ok := strings.Cut(p, ".")
			if !ok {
				continue
			}
			idx, e := strconv.Atoi(idxText)
			if e != nil || idx < 0 {
				continue
			}
			if pending[idx] == nil {
				pending[idx] = &Preset{}
			}
			switch field {
			case "fake-sni":
				pending[idx].FakeSNI = value
			case "upstream":
				pending[idx].Upstream = value
			}
			continue
		}
		switch key {
		case "listen": cfg.Listen = value
		case "connect": cfg.Connect = value
		case "fake-sni": cfg.FakeSNI = value
		case "utls": cfg.UTLS = value
		case "injector": cfg.Injector = value
		case "fake-repeat": if n, e := strconv.Atoi(value); e == nil { cfg.FakeRepeat = n }
		case "fake-delay": if n, e := parseDurationMs(value); e == nil { cfg.FakeDelayMs = n }
		case "ack-timeout": if n, e := parseDurationMs(value); e == nil { cfg.AckTimeoutMs = n }
		case "enable-fragment": if b, e := strconv.ParseBool(value); e == nil { cfg.EnableFragment = b }
		case "fragment-delay": if n, e := parseDurationMs(value); e == nil { cfg.FragmentDelayMs = n }
		case "sni-chunk": if n, e := strconv.Atoi(value); e == nil { cfg.SNIChunk = n }
		}
	}
	if err := s.Err(); err != nil {
		return SavedAppConfig{Config: cfg, Presets: presets}, err
	}
	for i := 0; i <= len(pending); i++ {
		if p := pending[i]; p != nil && strings.TrimSpace(p.FakeSNI) != "" && strings.TrimSpace(p.Upstream) != "" {
			loadedPresets = append(loadedPresets, *p)
		}
	}
	if len(loadedPresets) > 0 {
		presets = normalizePresets(loadedPresets)
	}
	return SavedAppConfig{Config: cfg, Presets: presets}, nil
}

func parseDurationMs(v string) (int, error) {
	v = strings.TrimSpace(strings.TrimSuffix(strings.ToLower(v), "ms"))
	return strconv.Atoi(v)
}

func normalizePresets(in []Preset) []Preset {
	out := make([]Preset, 0, len(in))
	seen := map[string]bool{}
	for _, p := range in {
		p.FakeSNI = strings.TrimSpace(p.FakeSNI)
		p.Upstream = strings.TrimSpace(p.Upstream)
		if p.FakeSNI == "" || p.Upstream == "" { continue }
		key := strings.ToLower(p.FakeSNI) + "\x00" + strings.ToLower(p.Upstream)
		if seen[key] { continue }
		seen[key] = true
		out = append(out, p)
	}
	return out
}

func SaveConfigFile(cfg ProxyConfig, presets []Preset) error {
	if err := guiapi.ValidateConfig(cfg); err != nil { return err }
	path, err := configINIPath()
	if err != nil { return err }
	presets = normalizePresets(presets)
	var b strings.Builder
	fmt.Fprintf(&b, "listen=%s\nconnect=%s\nfake-sni=%s\nutls=%s\ninjector=%s\nfake-repeat=%d\nfake-delay=%dms\nack-timeout=%dms\nenable-fragment=%t\nfragment-delay=%dms\nsni-chunk=%d\n\n[presets]\n", cfg.Listen, cfg.Connect, cfg.FakeSNI, cfg.UTLS, cfg.Injector, cfg.FakeRepeat, cfg.FakeDelayMs, cfg.AckTimeoutMs, cfg.EnableFragment, cfg.FragmentDelayMs, cfg.SNIChunk)
	for i, p := range presets {
		fmt.Fprintf(&b, "preset.%d.fake-sni=%s\npreset.%d.upstream=%s\n", i, p.FakeSNI, i, p.Upstream)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0644); err != nil { return err }
	if err := os.Rename(tmp, path); err != nil { _ = os.Remove(tmp); return err }
	return nil
}
