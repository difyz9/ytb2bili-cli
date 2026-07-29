package main

import "github.com/zolagz/ytb2bili-go/internal/cmd"

var Version = "dev"

func main() {
	cmd.Version = Version
	cmd.Execute()
}
