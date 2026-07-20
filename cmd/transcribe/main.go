package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/zolagz/ytb2bili-go/internal/transcriber"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "用法: go run cmd/transcribe/main.go <video.mp4>\n")
		os.Exit(1)
	}
	videoPath := os.Args[1]
	if _, err := os.Stat(videoPath); err != nil {
		fmt.Fprintf(os.Stderr, "文件不存在: %s\n", videoPath)
		os.Exit(1)
	}

	outputDir := "data/transcribe_output"
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "创建输出目录失败: %v\n", err)
		os.Exit(1)
	}

	// Use the base filename without extension as videoID
	videoID := filepath.Base(videoPath)
	videoID = videoID[:len(videoID)-len(filepath.Ext(videoID))]

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	fmt.Printf("🎤 开始使用 Bcut ASR 听录: %s\n", videoPath)
	fmt.Printf("📁 输出目录: %s\n", outputDir)
	fmt.Printf("📝 视频ID: %s\n", videoID)
	fmt.Println()

	start := time.Now()
	srtPath, err := transcriber.BcutASRContext(ctx, videoPath, outputDir, videoID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 听录失败: %v\n", err)
		os.Exit(1)
	}
	elapsed := time.Since(start)

	// Read and print the result
	srtData, err := os.ReadFile(srtPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取字幕文件失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Printf("✅ 听录完成! (耗时: %v)\n", elapsed.Round(time.Second))
	fmt.Printf("📄 字幕文件: %s\n", srtPath)
	fmt.Println()
	fmt.Println("=== 字幕内容 ===")
	fmt.Println(string(srtData))
}
