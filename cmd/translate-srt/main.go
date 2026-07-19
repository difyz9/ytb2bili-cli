package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/zolagz/ytb2bili-go/internal/translator"
)

func main() {
	input := flag.String("input", "", "input SRT file")
	output := flag.String("output", "", "output SRT file")
	flag.Parse()
	if *input == "" || *output == "" {
		flag.Usage()
		os.Exit(2)
	}
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "DEEPSEEK_API_KEY is required")
		os.Exit(2)
	}
	t := translator.New(translator.Config{
		APIKey: apiKey, BaseURL: "https://api.deepseek.com", Model: "deepseek-chat",
		SourceLang: "en", TargetLang: "zh", BatchSize: 3, MaxWorkers: 3, RetryCount: 3, ContextSize: 3,
	})
	if err := t.TranslateSRTFile(context.Background(), *input, *output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
