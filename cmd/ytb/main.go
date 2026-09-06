package main

import "github.com/zolagz/ytb2bili-go/internal/cli"

var Version = "dev"

func main() {
	cli.Version = Version
	cli.Execute()
}
