package main

import (
	"testing"

	"github.com/zolagz/ytb2bili-go/internal/config"
)

func TestCLIOptionsApplyConfig(t *testing.T) {
	options := cliOptions{input: "/tmp/video.en.srt", sourceLang: "en"}
	options.applyConfig(&config.Config{
		LLMBaseURL:            "https://llm.example",
		LLMModel:              "translation-model",
		TranslationTargetLang: "zh-Hans",
	})
	if options.output != "/tmp/video.zh-Hans.srt" {
		t.Fatalf("output=%q", options.output)
	}
	if options.targetLang != "zh-Hans" || options.baseURL != "https://llm.example" || options.model != "translation-model" {
		t.Fatalf("unexpected resolved options: %#v", options)
	}
}

func TestCLIOptionsValidateBatchSettings(t *testing.T) {
	valid := cliOptions{input: "input.srt", batchSize: 25, maxWorkers: 3, retryCount: 2, contextSize: 2}
	if err := valid.validate(); err != nil {
		t.Fatalf("valid options rejected: %v", err)
	}
	invalid := valid
	invalid.contextSize = -1
	if err := invalid.validate(); err == nil {
		t.Fatal("negative context size was accepted")
	}
}
