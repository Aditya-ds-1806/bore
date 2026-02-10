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
	LogFile string
}

func ParseFlags() Flags {
	version := flag.Bool("version", false, "Show application version")
	flag.BoolVar(version, "v", false, "Show application version")

	port := flag.Int("port", 8080, "Port to run the server on")
	flag.IntVar(port, "p", 8080, "Port to run the server on")

	logFile := flag.String("log-file", "./logs/bore.log", "Log file path")
	flag.StringVar(logFile, "l", "./logs/bore.log", "Log file path")

	flag.Parse()

	return Flags{
		Version: *version,
		Port:    *port,
		LogFile: *logFile,
	}
}

func main() {
	flags := ParseFlags()

	if flags.Version {
		fmt.Println("bore-server:", AppVersion)
		return
	}

	bs := server.NewBoreServer(&server.BoreServerCfg{
		Port:    flags.Port,
		LogFile: flags.LogFile,
		Version: AppVersion,
	})

	err := bs.StartBoreServer()
	if err != nil {
		fmt.Println("Failed to start bore server")
		panic(err)
	}
}
