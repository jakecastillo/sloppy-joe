// Package config loads rules and signals from disk for the CLI and daemon.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sloppyjoe/sloppy/core"
	"github.com/sloppyjoe/sloppy/rules"
	"github.com/sloppyjoe/sloppy/state"
)

// OpenStore opens the configured state backend ("sqlite" default, or "redis").
// Optional StoreOptions (e.g. state.WithCheckpointSigner) are forwarded to the
// backend so callers can enable the signed audit checkpoint without OpenStore
// needing to know about signing.
func OpenStore(kind, sqlitePath, redisAddr string, opts ...state.StoreOption) (state.Store, error) {
	switch kind {
	case "", "sqlite":
		st, err := state.OpenSQLite(sqlitePath, opts...)
		if err != nil {
			return nil, humanizeSQLiteOpen(sqlitePath, err)
		}
		return st, nil
	case "redis":
		if redisAddr == "" {
			return nil, errors.New("config: redis store requires a redis address")
		}
		return state.OpenRedis(redisAddr, opts...)
	default:
		return nil, fmt.Errorf("config: unknown store %q (want sqlite|redis)", kind)
	}
}

// humanizeSQLiteOpen turns the SQLite driver's terse open failures into a
// friendly, path-named error and drops the raw "(14)"/"(26)" extended-code
// text. SQLite reports the two common operator mistakes the same terse way:
//
//	"unable to open database file (14)"   SQLITE_CANTOPEN — usually a missing parent dir
//	"file is not a database (26)"          SQLITE_NOTADB   — the file exists but isn't a sloppy db
//
// We classify by inspecting the filesystem (the parent directory and the file
// itself) rather than parsing the numeric code, so the remedy we suggest names
// the actual cause.
func humanizeSQLiteOpen(path string, err error) error {
	// Missing parent directory: the most common cause of "unable to open
	// database file" — sqlite won't create intermediate dirs.
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if _, statErr := os.Stat(dir); errors.Is(statErr, os.ErrNotExist) {
			return fmt.Errorf("state db %s: parent directory %s does not exist; "+
				"create it first, then re-run", path, dir)
		}
	}
	// File exists but the driver rejected it: a non-sloppy / corrupt file
	// sitting at the db path (e.g. a text file or truncated db).
	if isNotADatabase(err) {
		return fmt.Errorf("state db %s: not a sloppy state database "+
			"(remove or rename this file, then re-run to create a fresh one)", path)
	}
	// Anything else: still name the db path so the operator knows which file
	// failed, but keep the underlying cause for diagnosis.
	return fmt.Errorf("state db %s: %w", path, err)
}

// isNotADatabase reports whether err is SQLite's SQLITE_NOTADB ("file is not a
// database"), matched on the driver's stable message text so config need not
// import the sqlite driver just to read an extended result code.
func isNotADatabase(err error) bool {
	return strings.Contains(err.Error(), "file is not a database")
}

// LoadRules reads a rules file or every *.yaml/*.yml in a directory (one rule per file).
func LoadRules(path string) ([]rules.Rule, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Drop the raw OS syscall text (e.g. "GetFileAttributesEx rules:
			// The system cannot find the path specified.") for a path-named,
			// actionable message.
			return nil, fmt.Errorf("rules path %s not found; create it and add "+
				"*.yaml rule files, or point at an existing one with --rules <dir|file>", path)
		}
		return nil, fmt.Errorf("rules path %s: %w", path, err)
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
			return nil, fmt.Errorf("rules file %s: %w", f, err)
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
		if errors.Is(err, os.ErrNotExist) {
			// Drop the raw "open <path>: The system cannot find..." OS text.
			return core.Signal{}, fmt.Errorf("signal file %s not found", path)
		}
		return core.Signal{}, fmt.Errorf("signal file %s: %w", path, err)
	}
	var s core.Signal
	if err := json.Unmarshal(b, &s); err != nil {
		// Name the file: a bare "invalid character ..." gives the operator no
		// clue which input was malformed.
		return core.Signal{}, fmt.Errorf("signal file %s: malformed JSON: %w", path, err)
	}
	return s, nil
}

// LoadSignalsJSONL reads a JSONL file (one core.Signal per non-empty line).
func LoadSignalsJSONL(path string) ([]core.Signal, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("signal file %s not found", path)
		}
		return nil, fmt.Errorf("signal file %s: %w", path, err)
	}
	var sigs []core.Signal
	for i, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var s core.Signal
		if err := json.Unmarshal([]byte(line), &s); err != nil {
			return nil, fmt.Errorf("signal file %s line %d: malformed JSON: %w", path, i+1, err)
		}
		sigs = append(sigs, s)
	}
	if len(sigs) == 0 {
		return nil, fmt.Errorf("no signals in %s", path)
	}
	return sigs, nil
}
