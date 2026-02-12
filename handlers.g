// internal/httpapi/handlers.go
package httpapi

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"alerts-api/internal/sysdig"
	"alerts-api/internal/templates"
	"alerts-api/internal/tokens"
)

type Handler struct {
	tokens *tokens.Store
	sysdig *sysdig.Client

	catalog  *templates.Catalog
	renderer *templates.Renderer

	defaultTeamIndex int
}

func NewHandler(tok *tokens.Store, sd *sysdig.Client, catalog *templates.Catalog, renderer *templates.Renderer, defaultTeamIndex int) *Handler {
	return &Handler{
		tokens:            tok,
		sysdig:            sd,
		catalog:           catalog,
		renderer:          renderer,
		defaultTeamIndex:  defaultTeamIndex,
	}
}

type StandardAlertRequest struct {
	IbmAccount          string   `json:"ibmAccount"`
	Application         string   `json:"application"`
	InstanceOrNamespace string   `json:"instanceOrNamespace"`
	CodeAP              string   `json:"codeAP"`

	// NEW: liste exacte des alertes (fichiers .json) à créer
	// Ex: ["connections_high.json","cpu_high.json"]
	Alerts []string `json:"alerts,omitempty"`
}

func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// GET /v1/alerts/catalog?application=PostgreSql
func (h *Handler) ListCatalog(w http.ResponseWriter, r *http.Request) {
	app := strings.TrimSpace(r.URL.Query().Get("application"))
	if app == "" {
		http.Error(w, "missing query param: application", http.StatusBadRequest)
		return
	}

	defs, err := h.catalog.LoadAlertDefs(app)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	type item struct {
		File     string `json:"file"`
		Severity string `json:"severity,omitempty"`
		Value    string `json:"value,omitempty"`
		Enabled  string `json:"enabled,omitempty"`
	}

	out := make([]item, 0, len(defs))
	for _, d := range defs {
		out = append(out, item{
			File:     d.File,
			Severity: d.Severity,
			Value:    d.Value,
			Enabled:  d.Enabled,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"application": app,
		"appDir":      h.catalog.AppDir(app),
		"count":       len(out),
		"available":   out,
	})
}

// POST /v1/alerts/standard
func (h *Handler) CreateStandardAlert(w http.ResponseWriter, r *http.Request) {
	var req StandardAlertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body: "+err.Error(), http.StatusBadRequest)
		return
	}

	req.IbmAccount = strings.TrimSpace(req.IbmAccount)
	req.Application = strings.TrimSpace(req.Application)
	req.InstanceOrNamespace = strings.TrimSpace(req.InstanceOrNamespace)
	req.CodeAP = strings.TrimSpace(req.CodeAP)

	if req.IbmAccount == "" || req.Application == "" || req.InstanceOrNamespace == "" || req.CodeAP == "" {
		http.Error(w, "missing required fields: ibmAccount, application, instanceOrNamespace, codeAP", http.StatusBadRequest)
		return
	}

	// 1) load token info (instanceID + IAM token + team IDs)
	tok, err := h.tokens.LoadForAccount(req.IbmAccount)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 2) team default OPS (selon ton store.go modifié)
	teamID, _ := tok.TeamIDAuto(req.InstanceOrNamespace, req.CodeAP, h.defaultTeamIndex)

	// 3) load catalog csv (all defs for this application)
	defsAll, err := h.catalog.LoadAlertDefs(req.Application)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 4) select exact alerts if provided
	defs := defsAll
	if len(req.Alerts) > 0 {
		selected, missing := filterDefs(defsAll, req.Alerts)
		if len(missing) > 0 {
			http.Error(w, "unknown alerts: "+strings.Join(missing, ", "), http.StatusBadRequest)
			return
		}
		if len(selected) == 0 {
			http.Error(w, "no alerts selected", http.StatusBadRequest)
			return
		}
		defs = selected
	}

	// 5) base vars for placeholders
	varsBase := map[string]string{
		"INSTANCE": req.InstanceOrNamespace,
		"CODEAP":   req.CodeAP,
		"CODE_AP":  req.CodeAP,
		"TYPE":     req.Application,
	}

	// 6) create alerts
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	type result struct {
		File   string         `json:"file"`
		Status string         `json:"status"`
		Error  string         `json:"error,omitempty"`
		Sysdig map[string]any `json:"sysdig,omitempty"`
	}

	results := make([]result, 0, len(defs))

	for _, d := range defs {
		// vars per-alert (csv can override VALUE/SEVERITY/ENABLED)
		vars := make(map[string]string, len(varsBase)+3)
		for k, v := range varsBase {
			vars[k] = v
		}
		if d.Value != "" {
			vars["VALUE"] = d.Value
		}
		if d.Severity != "" {
			vars["SEVERITY"] = d.Severity
		}
		if d.Enabled != "" {
			vars["ENABLED"] = d.Enabled
		}

		alertBody, err := h.renderer.RenderFromFile(req.Application, d.File, vars)
		if err != nil {
			results = append(results, result{File: d.File, Status: "render_error", Error: err.Error()})
			continue
		}

		log.Printf("CREATE alert app=%s file=%s teamID=%s", req.Application, d.File, teamID)
		log.Printf("ALERT_BODY=%s", string(alertBody))

		out, err := h.sysdig.CreateAlert(ctx, tok.IbmInstanceID, teamID, tok.IamToken, alertBody)
		if err != nil {
			results = append(results, result{File: d.File, Status: "create_error", Error: err.Error()})
			continue
		}
		results = append(results, result{File: d.File, Status: "created", Sysdig: out})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":     "done",
		"ibmAccount": req.IbmAccount,
		"application": req.Application,
		"instance":   req.InstanceOrNamespace,
		"codeAP":     req.CodeAP,
		"teamIdUsed": teamID,
		"count":      len(results),
		"results":    results,
	})
}

func filterDefs(defs []templates.AlertDef, wanted []string) ([]templates.AlertDef, []string) {
	// index all defs by file name (case-insensitive)
	idx := make(map[string]templates.AlertDef, len(defs))
	for _, d := range defs {
		k := strings.ToLower(strings.TrimSpace(d.File))
		if k != "" {
			idx[k] = d
		}
	}

	selected := make([]templates.AlertDef, 0, len(wanted))
	missing := make([]string, 0)

	for _, w := range wanted {
		key := strings.ToLower(strings.TrimSpace(w))
		if key == "" {
			continue
		}
		d, ok := idx[key]
		if !ok {
			missing = append(missing, w)
			continue
		}
		selected = append(selected, d)
	}

	return selected, missing
}
