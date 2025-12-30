package server

import (
	"bore/internal/web/logger"
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/google/uuid"
	"resty.dev/v3"
)

type ProxyServerConfig struct {
	UpstreamURL string
	Logger      *logger.Logger
}

type ProxyServer struct {
	server      *http.Server
	resty       *resty.Client
	Logger      *logger.Logger
	UpstreamURL string
}

func (s *ProxyServer) StartProxy() error {
	defer s.resty.Close()
	return s.server.ListenAndServe()
}

func NewProxyServer(cfg *ProxyServerConfig) *ProxyServer {
	resty := resty.New().SetBaseURL(cfg.UpstreamURL)

	server := http.Server{
		Addr: ":8080",
		Handler: (http.HandlerFunc)(func(w http.ResponseWriter, r *http.Request) {
			requestId := uuid.New().String()

			fmt.Println("Received request:", requestId, r.Method, r.URL.Path, "from", r.RemoteAddr)

			// Remove hop-by-hop headers
			headers := r.Header.Clone()
			headers.Del("Connection")
			headers.Del("Keep-Alive")
			headers.Del("Proxy-Authenticate")
			headers.Del("Proxy-Authorization")
			headers.Del("TE")
			headers.Del("Transfer-Encoding")
			headers.Del("Upgrade")
			headers.Del("Trailer")

			// Set Accept-Encoding strictly to gzip and deflate, brotli not supported by resty
			headers.Set("Accept-Encoding", "gzip, deflate")

			ctx := context.WithValue(r.Context(), logger.RequestIDKey, requestId)

			req := resty.
				SetContext(ctx).
				NewRequest().
				SetMethod(r.Method).
				SetURL(r.URL.Path).
				SetQueryParamsFromValues(r.URL.Query()).
				SetBody(r.Body).
				SetCookies(r.Cookies()).
				SetHeaderMultiValues(headers).
				SetHeader("X-Forwarded-For", r.RemoteAddr)

			fmt.Println("Curl", r.Body)

			cfg.Logger.LogRequest(req)

			res, err := req.Send()
			if err != nil {
				fmt.Println("Error fetching data:", err)
				http.Error(w, "Error fetching data", http.StatusInternalServerError)
				return
			}

			cfg.Logger.LogResponse(res)

			for key, values := range res.Header() {
				for _, value := range values {
					fmt.Println("Adding header:", key, value)
					w.Header().Add(key, value)
				}
			}

			w.WriteHeader(res.StatusCode())

			size, err := io.Copy(w, res.Body)
			if err != nil {
				fmt.Println("Error writing response:", err)
				http.Error(w, "Error writing response", http.StatusInternalServerError)
				return
			}

			w.(http.Flusher).Flush()

			fmt.Printf("Forwarded %d bytes to %s\n", size, r.RemoteAddr)
		}),
	}

	return &ProxyServer{
		UpstreamURL: cfg.UpstreamURL,
		Logger:      cfg.Logger,
		resty:       resty,
		server:      &server,
	}
}
