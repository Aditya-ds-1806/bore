package main

import (
	"flag"
	"fmt"
	"bore/internal/server"
	"os"
	"sync"
)

func main() {
	var wg sync.WaitGroup

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

		err := server.StartProxy(*upstreamURL)
		if err != nil {
			panic(err)
		}
	}()

	fmt.Println("Server is running on http://localhost:8080")
	wg.Wait()
}
