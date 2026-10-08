package main

import (
	"log"
	"os"

	"github.com/example/control-plane/internal/node"
)

func main() {
	if len(os.Args) > 1 {
		var err error
		switch os.Args[1] {
		case "stop-managed-servers":
			err = node.StopManagedServers()
		case "disable-managed-server-restarts":
			err = node.DisableManagedServerRestarts()
		default:
			log.Fatalf("unknown command: %s", os.Args[1])
		}
		if err != nil {
			log.Fatal(err)
		}
		return
	}
	cfg := node.Config{PanelURL: os.Getenv("PANEL_URL"), NodeToken: os.Getenv("NODE_TOKEN"), Listen: os.Getenv("NODE_ADDR")}
	if cfg.Listen == "" {
		cfg.Listen = ":8090"
	}
	if cfg.PanelURL == "" || cfg.NodeToken == "" {
		log.Fatal("PANEL_URL and NODE_TOKEN are required")
	}
	if err := node.Run(cfg); err != nil {
		log.Fatal(err)
	}
}
