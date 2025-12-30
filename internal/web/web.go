package web

import (
	"bore/internal/web/logger"
	"fmt"
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
			fmt.Println(err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		err = tmpl.Execute(w, ws.Logger.GetLogs())

		if err != nil {
			fmt.Println(err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	})

	return http.ListenAndServe(":8000", nil)
}
