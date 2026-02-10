// internal/tokens/store.go
package tokens

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// D'après tes infos : ordre des teamIDs dans ./token
// 0 = Monitor Operations
// 1 = BP2I
// 2 = DEV
// 3 = OPS   <-- default demandé
// 4 = TMP
const defaultOpsIndex = 3

type Store struct {
	dir string

	mu    sync.RWMutex
	cache map[string]*Token // key: ibmAccount
}

type Token struct {
	IbmAccount    string   `json:"ibmAccount"`
	IamToken      string   `json:"iamToken"`
	IbmInstanceID string   `json:"ibmInstanceID"`
	TeamIDs       []string `json:"teamIDs"`
}

func NewStore(dir string) *Store {
	return &Store{
		dir:   dir,
		cache: make(map[string]*Token),
	}
}

// LoadForAccount charge le token correspondant à ibmAccount depuis s.dir.
// Supporte plusieurs conventions de noms de fichiers et fallback scan du dossier.
func (s *Store) LoadForAccount(ibmAccount string) (*Token, error) {
	ibmAccount = strings.TrimSpace(ibmAccount)
	if ibmAccount == "" {
		return nil, errors.New("ibmAccount vide")
	}

	// cache
	s.mu.RLock()
	if tok, ok := s.cache[ibmAccount]; ok && tok != nil {
		s.mu.RUnlock()
		return tok, nil
	}
	s.mu.RUnlock()

	// 1) noms directs
	candidates := []string{
		filepath.Join(s.dir, fmt.Sprintf("%s.json", ibmAccount)),
		filepath.Join(s.dir, fmt.Sprintf("ibmAccount-%s.json", ibmAccount)),
		filepath.Join(s.dir, fmt.Sprintf("token-%s.json", ibmAccount)),
	}

	for _, p := range candidates {
		if tok, err := readTokenFile(p); err == nil && tok != nil {
			if tok.IbmAccount == "" {
				tok.IbmAccount = ibmAccount
			}
			if tok.IbmAccount == ibmAccount {
				s.mu.Lock()
				s.cache[ibmAccount] = tok
				s.mu.Unlock()
				return tok, nil
			}
		}
	}

	// 2) scan dossier
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("impossible de lire le dossier tokens %q: %w", s.dir, err)
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".json") {
			continue
		}

		p := filepath.Join(s.dir, name)
		tok, err := readTokenFile(p)
		if err != nil || tok == nil {
			continue
		}
		if strings.TrimSpace(tok.IbmAccount) == ibmAccount {
			s.mu.Lock()
			s.cache[ibmAccount] = tok
			s.mu.Unlock()
			return tok, nil
		}
	}

	return nil, fmt.Errorf("aucun fichier token trouvé pour ibmAccount=%s dans %s", ibmAccount, s.dir)
}

func readTokenFile(path string) (*Token, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	b = trimBOMAndSpace(b)

	// objet
	var tok Token
	if err := json.Unmarshal(b, &tok); err == nil {
		if tok.IamToken == "" && tok.IbmInstanceID == "" && tok.IbmAccount == "" && len(tok.TeamIDs) == 0 {
			return nil, fmt.Errorf("token vide dans %s", path)
		}
		return &tok, nil
	}

	// liste
	var toks []Token
	if err := json.Unmarshal(b, &toks); err == nil && len(toks) > 0 {
		t := toks[0]
		return &t, nil
	}

	return nil, fmt.Errorf("json invalide dans %s", path)
}

// TeamIDAuto : UNIQUEMENT basé sur l'ordre des TeamIDs.
// - retourne OPS (index 3) par défaut
// - sinon fallback index 0
func (t *Token) TeamIDAuto() (string, error) {
	if len(t.TeamIDs) == 0 {
		return "", errors.New("aucun teamID dans le token (teamIDs vide)")
	}

	// default OPS
	if len(t.TeamIDs) > defaultOpsIndex {
		if id := strings.TrimSpace(t.TeamIDs[defaultOpsIndex]); id != "" {
			return id, nil
		}
	}

	// fallback index 0
	if id := strings.TrimSpace(t.TeamIDs[0]); id != "" {
		return id, nil
	}

	return "", errors.New("teamIDs présents mais vides (strings vides)")
}

// --- helpers ---

func trimBOMAndSpace(b []byte) []byte {
	// UTF-8 BOM: EF BB BF
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		b = b[3:]
	}
	return []byte(strings.TrimSpace(string(b)))
}
