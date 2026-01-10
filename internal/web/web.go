package web

import (
	"bore/internal/web/logger"
	"bytes"
	"html/template"
	"net/http"
)

type WebServer struct {
	Logger *logger.Logger
}

func (ws *WebServer) StartServer() error {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		tmpl, err := template.ParseFiles("internal/web/templates/index.html")
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
