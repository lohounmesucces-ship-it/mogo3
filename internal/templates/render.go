package templates

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Renderer struct{ dir string }

func NewRenderer(dir string) *Renderer { return &Renderer{dir: dir} }

// Render charge templates/<application>.json (application en minuscule)
// et remplace ##KEY## par vars["KEY"].
func (r *Renderer) Render(application string, vars map[string]string) ([]byte, error) {
	filename := strings.ToLower(application) + ".json"
	path := filepath.Join(r.dir, filename)

	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read template %s: %w", path, err)
	}

	out := string(b)
	for k, v := range vars {
		out = strings.ReplaceAll(out, "##"+k+"##", v)
	}
	return []byte(out), nil
}
