package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// LoadExportFile lit un fichier bash contenant des lignes: export KEY=VALUE
// Ignore les commentaires (#), lignes vides, et les lignes non "export".
func LoadExportFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.HasPrefix(line, "export ") {
			continue
		}

		kv := strings.TrimSpace(strings.TrimPrefix(line, "export "))
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		val = strings.Trim(val, `"'`)

		_ = os.Setenv(key, val)
	}

	if err := sc.Err(); err != nil {
		return fmt.Errorf("scan %s: %w", path, err)
	}
	return nil
}
