package config

import (
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

// RenderEffective writes the effective configuration as YAML. It NEVER resolves
// token_env values (the broker is not consulted), so a resolved secret cannot leak
// into terminal/log/CI output. When showProvenance is set, a trailing comment block
// lists each tracked key's source, highlighting env/flag overrides as deviations
// from the committed file.
func RenderEffective(w io.Writer, e Effective, showProvenance bool) error {
	b, err := yaml.Marshal(e.File)
	if err != nil {
		return err
	}
	if _, err := w.Write(b); err != nil {
		return err
	}
	if !showProvenance {
		return nil
	}
	if _, err := fmt.Fprintln(w, "\n# sources (precedence: flag > env > file > default):"); err != nil {
		return err
	}
	for _, k := range e.Keys() {
		src := e.prov[k]
		marker := ""
		if src == SourceEnv || src == SourceFlag {
			marker = "   <- overrides file"
		}
		if _, err := fmt.Fprintf(w, "#   %-28s %s%s\n", k, src, marker); err != nil {
			return err
		}
	}
	return nil
}
