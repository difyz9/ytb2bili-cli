package main

import (
	"fmt"
	"log"
	"os"

	"github.com/difyz9/ytb2bili-cli/internal/config"
	"github.com/difyz9/ytb2bili-cli/internal/command"
)

var Version = "dev"

func main() {
	cfg := config.Default()
	cfg.Init()

	app := command.NewApp(cfg)
	app.Version = Version

	if err := app.Run(os.Args); err != nil {
		log.Fatalf("❌ %v", err)
	}
}

func init() {
	// Print verbose version info
	_ = fmt.Sprintf
}
