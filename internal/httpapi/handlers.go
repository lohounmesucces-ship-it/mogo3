package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"alerts-api/internal/sysdig"
	"alerts-api/internal/templates"
	"alerts-api/internal/tokens"
)

type Handler struct {
	sysdig          *sysdig.Client
	tokens          *tokens.Store
	renderer        *templates.Renderer
	defaultTeamIndex int
}

func NewHandler(s *sysdig.Client, ts *tokens.Store, r *templates.Renderer, defaultTeamIndex int) *Handler {
	return &Handler{
		sysdig:          s,
		tokens:          ts,
		renderer:        r,
		defaultTeamIndex: defaultTeamIndex,
	}
}

type CreateRequest struct {
	IbmAccount          string `json:"ibmAccount"`
	Application         string `json:"application"`
	InstanceOrNamespace string `json:"instanceOrNamespace"`
	CodeAP              string `json:"codeAP"`
}

func (h *Handler) CreateStandardAlert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.IbmAccount == "" || req.Application == "" || req.InstanceOrNamespace == "" || req.CodeAP == "" {
		http.Error(w, "missing fields: ibmAccount, application, instanceOrNamespace, codeAP", http.StatusBadRequest)
		return
	}

	// 1) charge token du compte
	tok, err := h.tokens.LoadForAccount(req.IbmAccount)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 2) choisit teamID automatiquement (sans mapping)
	teamID, detectedEnv := tok.TeamIDAuto(req.InstanceOrNamespace, req.CodeAP, h.defaultTeamIndex)
	if teamID == "" {
		http.Error(w, "no teamID available in token file", http.StatusBadRequest)
		return
	}

	// 3) render template JSON
	vars := map[string]string{
		"IBM_ACCOUNT": req.IbmAccount,
		"APP":         req.Application,
		"INSTANCE":    req.InstanceOrNamespace,
		"CODE_AP":     req.CodeAP,
		"TEAM_ID":     teamID, // si tu veux l’injecter dans le JSON
	}
	alertBody, err := h.renderer.Render(req.Application, vars)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 4) call sysdig
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	out, err := h.sysdig.CreateAlert(ctx, tok.IbmInstanceID, teamID, tok.IamToken, alertBody)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":      "created",
		"ibmAccount":   req.IbmAccount,
		"application":  req.Application,
		"instance":     req.InstanceOrNamespace,
		"codeAP":       req.CodeAP,
		"envDetected":  detectedEnv,
		"teamIDUsed":   teamID,
		"sysdigResult": out,
	})
}
