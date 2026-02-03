package main

import (
	"bore/internal/server"
	"flag"
	"fmt"
)

var AppVersion string

type Flags struct {
	Version bool
	Port    int
}

func ParseFlags() Flags {
	version := flag.Bool("version", false, "Show application version")
	flag.BoolVar(version, "v", false, "Show application version")

	port := flag.Int("port", 8080, "Port to run the server on")
	flag.IntVar(port, "p", 8080, "Port to run the server on")

	flag.Parse()

	return Flags{
		Version: *version,
		Port:    *port,
	}
}

func main() {
	flags := ParseFlags()

	if flags.Version {
		fmt.Println("bore-server:", AppVersion)
		return
	}

	bs := server.NewBoreServer(&server.BoreServerCfg{
		Port: flags.Port,
	})

	err := bs.StartBoreServer()
	if err != nil {
		fmt.Println("Failed to start bore server")
		panic(err)
	}
}
