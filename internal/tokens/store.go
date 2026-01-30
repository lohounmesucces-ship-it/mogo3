package tokens

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type TokenInfo struct {
	IbmInstanceID string
	IamToken      string
	TeamIDs       []string // envs: DEV, QUALIF, PP, PROD, DR (par convention)
}

type Store struct {
	BaseDir string // /apps/sysdig/data-save/scripts/.token
}

func NewStore(baseDir string) *Store { return &Store{BaseDir: baseDir} }

// Charge: BaseDir/<ibmAccount>/token
// Format: GUID;TOKEN;team1;team2;team3;team4;team5
func (s *Store) LoadForAccount(ibmAccount string) (TokenInfo, error) {
	p := filepath.Join(s.BaseDir, ibmAccount, "token")

	b, err := os.ReadFile(p)
	if err != nil {
		return TokenInfo{}, fmt.Errorf("read token file %s: %w", p, err)
	}

	info := parseSemicolonLine(string(b))
	if info.IbmInstanceID == "" || info.IamToken == "" || len(info.TeamIDs) == 0 {
		return TokenInfo{}, fmt.Errorf("bad token format in %s (expected: GUID;TOKEN;TEAMID...)", p)
	}

	if !strings.HasPrefix(strings.ToLower(info.IamToken), "bearer ") {
		info.IamToken = "Bearer " + info.IamToken
	}
	return info, nil
}

func parseSemicolonLine(content string) TokenInfo {
	line := ""
	for _, l := range strings.Split(content, "\n") {
		l = strings.TrimSpace(l)
		if l != "" && !strings.HasPrefix(l, "#") {
			line = l
			break
		}
	}
	if line == "" {
		return TokenInfo{}
	}

	raw := strings.Split(line, ";")
	parts := make([]string, 0, len(raw))
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) < 3 {
		return TokenInfo{}
	}
	return TokenInfo{
		IbmInstanceID: parts[0],
		IamToken:      parts[1],
		TeamIDs:       parts[2:],
	}
}

// ----- Auto env detection (sans champ env) -----

type Env string

const (
	EnvUnknown Env = "UNKNOWN"
	EnvDev     Env = "DEV"
	EnvQualif  Env = "QUALIF"
	EnvPP      Env = "PP"
	EnvProd    Env = "PROD"
	EnvDR      Env = "DR"
)

// Ajuste les patterns si vos namespaces ont une convention spécifique
func InferEnv(instanceOrNamespace, codeAP string) Env {
	s := strings.ToLower(instanceOrNamespace + " " + codeAP)

	switch {
	case has(s, `\b(production|prod|prd)\b`):
		return EnvProd
	case has(s, `\b(pp|preprod|pre-prod|staging|stage)\b`):
		return EnvPP
	case has(s, `\b(qualif|qa|uat|test)\b`):
		return EnvQualif
	case has(s, `\b(dev|develop|sandbox)\b`):
		return EnvDev
	case has(s, `\b(dr|disaster|backup)\b`):
		return EnvDR
	default:
		return EnvUnknown
	}
}

func has(s, pattern string) bool {
	return regexp.MustCompile(pattern).FindStringIndex(s) != nil
}

// Convention d'ordre des TeamIDs dans le token : DEV, QUALIF, PP, PROD, DR
// Si l'env ne peut pas être détecté -> fallback sur DEFAULT_TEAM_INDEX (ou 0)
func (t TokenInfo) TeamIDAuto(instanceOrNamespace, codeAP string, defaultIndex int) (teamID string, detected Env) {
	env := InferEnv(instanceOrNamespace, codeAP)

	order := map[Env]int{
		EnvDev:    0,
		EnvQualif: 1,
		EnvPP:     2,
		EnvProd:   3,
		EnvDR:     4,
	}

	if idx, ok := order[env]; ok && idx >= 0 && idx < len(t.TeamIDs) {
		return t.TeamIDs[idx], env
	}

	// fallback
	if defaultIndex < 0 {
		defaultIndex = 0
	}
	if defaultIndex >= len(t.TeamIDs) {
		defaultIndex = 0
	}
	if len(t.TeamIDs) > 0 {
		return t.TeamIDs[defaultIndex], EnvUnknown
	}
	return "", EnvUnknown
}
