package pipeline

import (
	"github.com/zolagz/ytb2bili-go/internal/download"
	"github.com/zolagz/ytb2bili-go/internal/metadata"
)

// PipelineState is the strongly typed data exchanged by workflow steps.
type PipelineState struct {
	Request  Request
	Result   *Result
	Download *download.Result
	Metadata *metadata.VideoMeta
	AudioDir string // TTS 生成的配音片段目录，供 audio-sync 使用
}
