// Package config loads rules and signals from disk for the CLI and daemon.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sloppyjoe/sloppy/core"
	"github.com/sloppyjoe/sloppy/rules"
)

// LoadRules reads a rules file or every *.yaml/*.yml in a directory (one rule per file).
func LoadRules(path string) ([]rules.Rule, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	var files []string
	if info.IsDir() {
		entries, _ := os.ReadDir(path)
		for _, en := range entries {
			if en.IsDir() {
				continue
			}
			if ext := filepath.Ext(en.Name()); ext == ".yaml" || ext == ".yml" {
				files = append(files, filepath.Join(path, en.Name()))
			}
		}
	} else {
		files = []string{path}
	}
	var all []rules.Rule
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		rs, err := rules.ParseRules(b)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		all = append(all, rs...)
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("no rules found in %s", path)
	}
	return all, nil
}

// LoadSignal reads a JSON file into a core.Signal.
func LoadSignal(path string) (core.Signal, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return core.Signal{}, err
	}
	var s core.Signal
	if err := json.Unmarshal(b, &s); err != nil {
		return core.Signal{}, err
	}
	return s, nil
}

// LoadSignalsJSONL reads a JSONL file (one core.Signal per non-empty line).
func LoadSignalsJSONL(path string) ([]core.Signal, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sigs []core.Signal
	for i, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var s core.Signal
		if err := json.Unmarshal([]byte(line), &s); err != nil {
			return nil, fmt.Errorf("%s line %d: %w", path, i+1, err)
		}
		sigs = append(sigs, s)
	}
	if len(sigs) == 0 {
		return nil, fmt.Errorf("no signals in %s", path)
	}
	return sigs, nil
}
