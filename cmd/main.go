package main

import (
	"bore/internal/client"
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

		bs := server.NewBoreServer()

		fmt.Println("Bore server is running on http://localhost:8080/")

		err := bs.StartBoreServer()
		if err != nil {
			fmt.Println("Failed to start bore server")
			panic(err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		bc := client.NewBoreClient(&client.BoreClientConfig{
			UpstreamURL: *upstreamURL,
			Logger:      logger,
		})

		fmt.Println("Bore client is running")

		err := bc.StartBoreClient()
		if err != nil {
			fmt.Println("Failed to start bore client")
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
			fmt.Println("Failed to start bore client")
			panic(err)
		}
	}()

	wg.Wait()
}
