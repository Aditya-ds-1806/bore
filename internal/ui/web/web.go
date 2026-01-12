package web

import (
	"bore/internal/client"
	"bytes"
	"html/template"
	"net/http"
	"path/filepath"
)

type WebServer struct {
	Logger *client.Logger
}

func (ws *WebServer) StartServer() error {
	templatesDir := "internal/ui/web/templates"

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		templatePath := filepath.Join(templatesDir, "index.html")
		tmpl, err := template.ParseFiles(templatePath)
		if err != nil {
			// fmt.Println(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var buf bytes.Buffer
		err = tmpl.Execute(&buf, ws.Logger.GetLogs())
		if err != nil {
			// fmt.Println(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if _, err = buf.WriteTo(w); err != nil {
			// fmt.Println(err)
			return
		}
	})

	return http.ListenAndServe(":8000", nil)
}
