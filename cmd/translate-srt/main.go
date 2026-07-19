package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/zolagz/ytb2bili-go/internal/config"
	"github.com/zolagz/ytb2bili-go/internal/translator"
)

func main() {
	input := flag.String("input", "", "input SRT file")
	output := flag.String("output", "", "output SRT file (default: append target language)")
	configPath := flag.String("config", "config.yaml", "project YAML config")
	sourceLang := flag.String("source-lang", "en", "source language")
	targetLang := flag.String("target-lang", "", "target language (default: config translation_target_lang)")
	flag.Parse()
	if *input == "" {
		flag.Usage()
		os.Exit(2)
	}
	appConfig, err := config.LoadYAML(*configPath)
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "load config: %v\n", err)
			os.Exit(2)
		}
		appConfig = config.Default()
		appConfig.Init()
	}
	if *targetLang == "" {
		*targetLang = appConfig.EffectiveTranslationTargetLang()
	}
	if *output == "" {
		*output = translator.TranslatedSRTPath(*input, *targetLang)
	}
	if appConfig.LLMAPIKey == "" {
		fmt.Fprintln(os.Stderr, "DEEPSEEK_API_KEY is required")
		os.Exit(2)
	}
	t := translator.New(translator.Config{
		APIKey: appConfig.LLMAPIKey, BaseURL: appConfig.LLMBaseURL, Model: appConfig.LLMModel,
		SourceLang: *sourceLang, TargetLang: *targetLang, BatchSize: 3, MaxWorkers: 3, RetryCount: 3, ContextSize: 3,
	})
	if err := t.TranslateSRTFile(context.Background(), *input, *output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
