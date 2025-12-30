package main

import (
	"bore/internal/server"
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

		err := proxy.StartProxy()
		if err != nil {
			panic(err)
		}
	}()

	fmt.Println("Server is running on http://localhost:8080")
	wg.Wait()
}
