package server

import (
	"fmt"
	"io"
	"net/http"

	"resty.dev/v3"
)

func StartProxy(upstreamURL string) error {
	resty := resty.New().SetBaseURL(upstreamURL)
	defer resty.Close()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("Received request:", r.Method, r.URL.Path, "from", r.RemoteAddr)

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

		req := resty.
			SetContext(r.Context()).
			NewRequest().
			SetMethod(r.Method).
			SetURL(r.URL.Path).
			SetQueryParamsFromValues(r.URL.Query()).
			SetBody(r.Body).
			SetCookies(r.Cookies()).
			SetHeaderMultiValues(headers).
			SetHeader("X-Forwarded-For", r.RemoteAddr)

		res, err := req.Send()
		if err != nil {
			fmt.Println("Error fetching data:", err)
			http.Error(w, "Error fetching data", http.StatusInternalServerError)
			return
		}

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
	})

	return http.ListenAndServe(":8080", nil)
}
