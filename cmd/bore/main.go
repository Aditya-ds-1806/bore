package main

import (
	"bore/internal/client"
	"bore/internal/traffik"
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
	UpstreamURL   string
	Inspect       bool
	Debug         bool
	InspectPort   int
	allowExternal bool
}

func ParseFlags() Flags {
	version := flag.Bool("version", false, "Show application version")
	flag.BoolVar(version, "v", false, "Show application version")

	upstreamURL := flag.String("url", "", "Upstream URL to proxy requests to")
	flag.StringVar(upstreamURL, "u", "", "Upstream URL to proxy requests to")

	debug := flag.Bool("debug", false, "Enable debug mode (logs internal bore logs to a file)")
	flag.BoolVar(debug, "d", false, "Enable debug mode (logs internal bore logs to a file)")

	inspectPort := flag.Int("inspect-port", 8000, "Port to run the web inspector")
	inspect := flag.Bool("inspect", true, "Enable the web inspector")
	allowExternal := flag.Bool("allow-external", false, "Allow proxying non-localhost targets (disabled by default)")

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
		UpstreamURL:   *upstreamURL,
		InspectPort:   *inspectPort,
		Inspect:       *inspect,
		allowExternal: *allowExternal,
		Debug:         *debug,
	}
}

func main() {
	var wg sync.WaitGroup
	defer wg.Wait()

	flags := ParseFlags()
	logger := traffik.NewLogger()

	bc := client.NewBoreClient(&client.BoreClientConfig{
		UpstreamURL:   flags.UpstreamURL,
		Traffik:        logger,
		AllowExternal: flags.allowExternal,
		DebugMode:     flags.Debug,
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

	portCh := make(chan int, 1)

	ws := web.WebServer{
		Logger: logger,
		Port:   flags.InspectPort,
		PortCh: portCh,
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

	p := tea.NewProgram(tui.NewModel(logger, bc.AppURL, portCh), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("failed to run TUI: %v", err)
		os.Exit(1)
	}
}
