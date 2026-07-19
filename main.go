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
	// 优先加载 config.yaml，不存在则使用默认值
	cfg, err := config.LoadYAML("config.yaml")
	if err != nil {
		cfg = config.Default()
		cfg.Init()
	}

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
