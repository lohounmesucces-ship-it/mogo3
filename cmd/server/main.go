package main

import (
	"log"
	"net/http"
	"os"
	"strconv"

	"alerts-api/internal/config"
	"alerts-api/internal/httpapi"
	"alerts-api/internal/sysdig"
	"alerts-api/internal/templates"
	"alerts-api/internal/tokens"
)

func main() {
	// 1) charge tes exports depuis le fichier variables (photo)
	varsFile := getenv("VARS_FILE", "/apps/sysdig/data-save/scripts/sysdig.variables")
	if err := config.LoadExportFile(varsFile); err != nil {
		log.Fatalf("load vars file: %v", err)
	}

	// 2) récupère SDC_URL depuis l'env chargée
	sdcURL := mustEnv("SDC_URL")

	// 3) store token base dir
	tokenBaseDir := getenv("TOKEN_BASE_DIR", "/apps/sysdig/data-save/scripts/.token")

	// 4) templates dir (alertes exportées en json)
	templatesDir := getenv("TEMPLATES_DIR", "templates")

	// 5) fallback si env non détecté: index team par défaut
	defaultTeamIndex := getenvInt("DEFAULT_TEAM_INDEX", 0)

	sysdigClient := sysdig.New(sdcURL)
	tokenStore := tokens.NewStore(tokenBaseDir)
	renderer := templates.NewRenderer(templatesDir)

	h := httpapi.NewHandler(sysdigClient, tokenStore, renderer, defaultTeamIndex)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("/v1/alerts/standard", h.CreateStandardAlert)

	addr := ":" + getenv("PORT", "8080")
	log.Printf("SDC_URL=%s", sdcURL)
	log.Printf("listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func getenv(k, def string) string {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	return v
}
func mustEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Fatalf("missing env var: %s", k)
	}
	return v
}
func getenvInt(k string, def int) int {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return i
}
