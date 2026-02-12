package web

import (
	"bore/internal/traffik"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"path/filepath"

	"github.com/go-chi/chi/v5"
)

const maxRetries int = 10

type WebServer struct {
	Traffik *traffik.Logger
	Port    int
	PortCh  chan<- int
}

func (ws *WebServer) StartServer() error {
	templatesDir := "internal/ui/web/templates"

	router := chi.NewRouter()

	router.Get("/api/logs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		filterQuery := r.URL.Query().Get("filter")

		logs, err := ws.Traffik.GetFilteredLogs(filterQuery)
		if err != nil {
			err := json.NewEncoder(w).Encode(map[string]any{
				"error": err.Error(),
				"logs":  nil,
			})

			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		summaries := make([]map[string]any, len(logs))
		for i, log := range logs {
			summaries[i] = map[string]any{
				"RequestID": log.RequestID,
				"Request": map[string]any{
					"method":    log.Request.Method,
					"path":      log.Request.Path,
					"timestamp": log.Request.Timestamp,
				},
				"Response": map[string]any{
					"status_code": log.Response.StatusCode,
					"timestamp":   log.Response.Timestamp,
				},
			}
		}

		err = json.NewEncoder(w).Encode(map[string]any{
			"error": nil,
			"logs":  summaries,
		})

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})

	router.Get("/api/logs/{requestID}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		requestID := chi.URLParam(r, "requestID")
		if requestID == "" {
			w.WriteHeader(http.StatusBadRequest)
			err := json.NewEncoder(w).Encode(map[string]any{
				"error": "Request ID is required",
			})

			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		log := ws.Traffik.GetLogByID(requestID)
		if log == nil {
			w.WriteHeader(http.StatusNotFound)
			err := json.NewEncoder(w).Encode(map[string]any{
				"error": "Log not found",
			})

			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		err := json.NewEncoder(w).Encode(map[string]any{
			"error": nil,
			"log":   log,
		})

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		templatePath := filepath.Join(templatesDir, "index.html")
		http.ServeFile(w, r, templatePath)
	})

	for range maxRetries {
		netListerner, err := net.Listen("tcp", fmt.Sprintf(":%d", ws.Port))
		if err == nil {
			ws.PortCh <- ws.Port
			close(ws.PortCh)
			return http.Serve(netListerner, router)
		}

		ws.Port++
	}

	return fmt.Errorf("failed to start web server after %d retries", maxRetries)
}
