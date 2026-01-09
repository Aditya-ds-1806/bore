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

var AppMode string

func RunBoreServer(wg *sync.WaitGroup) {
	defer wg.Done()

	bs := server.NewBoreServer()

	fmt.Println("Bore server is running on http://localhost:8080/")

	err := bs.StartBoreServer()
	if err != nil {
		fmt.Println("Failed to start bore server")
		panic(err)
	}
}

func RunBoreClient(logger *logger.Logger, wg *sync.WaitGroup) {
	defer wg.Done()

	upstreamURL := flag.String("url", "", "Upstream URL to proxy requests to")
	flag.StringVar(upstreamURL, "u", "", "Upstream URL to proxy requests to")

	flag.Parse()

	if *upstreamURL == "" {
		fmt.Println("Upstream URL is required. Use -url or -u to specify it.")
		os.Exit(1)
		return
	}

	bc := client.NewBoreClient(&client.BoreClientConfig{
		UpstreamURL: *upstreamURL,
		Logger:      logger,
	})

	go func() {
		for {
			if bc.AppId != nil {
				fmt.Println("You app is live on:", fmt.Sprintf("https://%s.%s", *bc.AppId, client.BoreServerHost))
				return
			}
		}
	}()

	err := bc.StartBoreClient()
	if err != nil {
		fmt.Println("Failed to start bore client")
		panic(err)
	}
}

func RunBoreWebClient(logger *logger.Logger, wg *sync.WaitGroup) {
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
}

func main() {
	var wg sync.WaitGroup

	if AppMode == "server" {
		wg.Add(1)
		go RunBoreServer(&wg)
	} else {
		var logger = logger.NewLogger()

		wg.Add(1)
		go RunBoreClient(logger, &wg)

		wg.Add(1)
		go RunBoreWebClient(logger, &wg)
	}

	wg.Wait()
}
