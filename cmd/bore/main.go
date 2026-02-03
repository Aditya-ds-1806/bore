package main

import (
	"bore/internal/client"
	"bore/internal/client/reqlogger"
	"bore/internal/ui/tui"
	"bore/internal/ui/web"
	"flag"
	"fmt"
	"os"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

var AppVersion string

type Flags struct {
	UpstreamURL string
	Inspect     bool
	InspectPort int
}

func ParseFlags() Flags {
	version := flag.Bool("version", false, "Show application version")
	flag.BoolVar(version, "v", false, "Show application version")

	upstreamURL := flag.String("url", "", "Upstream URL to proxy requests to")
	flag.StringVar(upstreamURL, "u", "", "Upstream URL to proxy requests to")

	inspectPort := flag.Int("inspect-port", 8000, "Port to run the web inspector")
	inspect := flag.Bool("inspect", true, "Enable the web inspector")

	flag.Parse()

	if *version {
		fmt.Println("bore:", AppVersion)
		os.Exit(0)
	}

	if *upstreamURL == "" {
		fmt.Println("Upstream URL is required. Use -url or -u to specify it.")
		os.Exit(1)
	}

	return Flags{
		UpstreamURL: *upstreamURL,
		InspectPort: *inspectPort,
		Inspect:     *inspect,
	}
}

func main() {
	var wg sync.WaitGroup
	defer wg.Wait()

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

	ws := web.WebServer{
		Logger: logger,
		Port:   flags.InspectPort,
	}

	if flags.Inspect {
		wg.Add(1)
		go func() {
			defer wg.Done()

			err := ws.StartServer()
			if err != nil {
				fmt.Println("Failed to start bore web client")
				panic(err)
			}
		}()
	}

	p := tea.NewProgram(tui.NewModel(logger, bc.AppURL, &ws.Port, flags.Inspect), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("failed to run TUI: %v", err)
		os.Exit(1)
	}
}
