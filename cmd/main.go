package main

import (
	"bore/internal/server"
	"bore/internal/web"
	"bore/internal/web/logger"
	"flag"
	"fmt"
	"os"
	"sync"
)

func main() {
	var wg sync.WaitGroup
	var logger = logger.NewLogger()

	upstreamURL := flag.String("url", "", "Upstream URL to proxy requests to")
	flag.StringVar(upstreamURL, "u", "", "Upstream URL to proxy requests to")

	flag.Parse()

	if *upstreamURL == "" {
		fmt.Println("Upstream URL is required. Use -url or -u to specify it.")
		os.Exit(1)
		return
	}

	wg.Add(1)
	go func() {
		defer wg.Done()

		proxy := server.NewProxyServer(&server.ProxyServerConfig{
			UpstreamURL: *upstreamURL,
			Logger:      logger,
		})

		fmt.Println("Proxy Server is running on http://localhost:8080")

		err := proxy.StartProxy()
		if err != nil {
			panic(err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		fmt.Println("Web Server is running on http://localhost:8000/")
		ws := web.WebServer{
			Logger: logger,
		}

		err := ws.StartServer()
		if err != nil {
			panic(err)
		}
	}()

	wg.Wait()
}
