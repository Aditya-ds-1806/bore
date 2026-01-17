package main

import (
	"bore/internal/server"
	"flag"
	"fmt"
	"sync"
)

var AppVersion string

type Flags struct {
	Version bool
}

func ParseFlags() Flags {
	version := flag.Bool("version", false, "Show application version")
	flag.BoolVar(version, "v", false, "Show application version")

	flag.Parse()

	return Flags{
		Version: *version,
	}
}

func main() {
	var wg sync.WaitGroup
	defer wg.Wait()

	flags := ParseFlags()

	if flags.Version {
		fmt.Println("bore-server:", AppVersion)
		return
	}

	wg.Add(1)
	bs := server.NewBoreServer()

	fmt.Println("Bore server is running on http://localhost:8080/")

	err := bs.StartBoreServer()
	if err != nil {
		fmt.Println("Failed to start bore server")
		panic(err)
	}
}
