package main

import (
	"fmt"
	"log"
	"os"

	"github.com/zolagz/ytb2bili-go/internal/config"
	"github.com/zolagz/ytb2bili-go/internal/command"
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
