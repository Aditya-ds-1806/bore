package main

import (
	"bore/internal/client"
	"bore/internal/client/reqlogger"
	"bore/internal/server"
	"bore/internal/ui/tui"
	"bore/internal/ui/web"
	"flag"
	"fmt"
	"os"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

var AppMode string

type Flags struct {
	UpstreamURL string
}

func ParseFlags() Flags {
	upstreamURL := flag.String("url", "", "Upstream URL to proxy requests to")
	flag.StringVar(upstreamURL, "u", "", "Upstream URL to proxy requests to")

	flag.Parse()

	if *upstreamURL == "" {
		fmt.Println("Upstream URL is required. Use -url or -u to specify it.")
		os.Exit(1)
	}

	return Flags{
		UpstreamURL: *upstreamURL,
	}
}

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

func RunBoreWebClient(logger *reqlogger.Logger, wg *sync.WaitGroup) {
	defer wg.Done()

	fmt.Println("Web Server is running on http://localhost:8000/")
	ws := web.WebServer{
		Logger: logger,
	}

	err := ws.StartServer()
	if err != nil {
		fmt.Println("Failed to start bore web client")
		panic(err)
	}
}

func main() {
	var wg sync.WaitGroup
	defer wg.Wait()

	if AppMode == "server" {
		wg.Add(1)
		go RunBoreServer(&wg)
		return
	}

	flags := ParseFlags()
	logger := reqlogger.NewLogger()

	bc := client.NewBoreClient(&client.BoreClientConfig{
		UpstreamURL: flags.UpstreamURL,
		Logger:      logger,
	})

	wg.Add(1)
	go func() {
		defer wg.Done()
		err := bc.RegisterApp()

		if err != nil {
			fmt.Printf("Failed to start bore client: %v\n", err)
			os.Exit(1)
		}
	}()

	<-bc.Ready

	wg.Add(1)
	go RunBoreWebClient(logger, &wg)

	p := tea.NewProgram(tui.NewModel(logger, bc.AppURL), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("failed to run TUI: %v", err)
		os.Exit(1)
	}
}
