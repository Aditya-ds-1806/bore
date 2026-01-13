package web

import (
	"bore/internal/client"
	"encoding/json"
	"net/http"
	"path/filepath"
)

type WebServer struct {
	Logger *client.Logger
}

func (ws *WebServer) StartServer() error {
	templatesDir := "internal/ui/web/templates"

	http.HandleFunc("/api/logs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		filterQuery := r.URL.Query().Get("filter")

		logs, err := ws.Logger.GetFilteredLogs(filterQuery)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]any{
				"error": err.Error(),
				"logs":  nil,
			})
			return
		}

		json.NewEncoder(w).Encode(map[string]any{
			"error": nil,
			"logs":  logs,
		})
	})

	// Main page endpoint
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		templatePath := filepath.Join(templatesDir, "index.html")
		http.ServeFile(w, r, templatePath)
	})

	return http.ListenAndServe(":8000", nil)
}
