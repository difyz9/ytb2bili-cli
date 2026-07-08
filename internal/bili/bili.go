package bili

import (
	"fmt"
	"os"
	"strings"

	"github.com/difyz9/bilibili-go-sdk/bilibili"

	"github.com/difyz9/ytb2bili-cli/internal/auth"
)

type UploadParams struct {
	VideoPath    string
	Title        string
	Desc         string
	Tags         []string
	Source       string
	Tid          int
	CoverPath    string
	SubtitlePath string
}

func Upload(cred *auth.LoginInfo, params *UploadParams) (string, error) {
	// Build SDK LoginInfo
	sdkLogin := CredToSDKLogin(cred)

	// Create clients
	client := bilibili.NewClient()
	uploadClient := bilibili.NewUploadClient(sdkLogin)

	// 1. Upload video file
	video, err := uploadClient.UploadVideo(params.VideoPath)
	if err != nil {
		return "", fmt.Errorf("上传视频文件失败: %w", err)
	}

	// 2. Upload cover
	coverURL := ""
	if params.CoverPath != "" {
		if _, err := os.Stat(params.CoverPath); err == nil {
			if url, err := uploadClient.UploadCover(params.CoverPath); err == nil {
				coverURL = url
			}
		}
	}

	// 3. Submit video
	tagStr := strings.Join(params.Tags, ",")
	studio := &bilibili.Studio{
		Copyright: 2,
		Source:    params.Source,
		Tid:       params.Tid,
		Cover:     coverURL,
		Title:     truncate(params.Title, 80),
		Desc:      params.Desc,
		Tag:       tagStr,
		Videos:    []bilibili.Video{*video},
		NoReprint: 0,
	}

	result, err := uploadClient.SubmitVideo(studio)
	if err != nil {
		return "", fmt.Errorf("投稿失败: %w", err)
	}

	bvid := extractBVID(result)
	if bvid == "" {
		return "", fmt.Errorf("投稿响应中未获取到BVID")
	}

	// 4. Upload subtitle
	if params.SubtitlePath != "" {
		if _, err := os.Stat(params.SubtitlePath); err == nil {
			lang := bilibili.NormalizeSubtitleLanguage("zh")
			if err := client.UploadSubtitle(sdkLogin, bvid, params.SubtitlePath, lang); err != nil {
				fmt.Fprintf(os.Stderr, "  ⚠ 字幕上传失败: %v\n", err)
			}
		}
	}

	return bvid, nil
}

func extractBVID(result *bilibili.ResponseData) string {
	if result == nil || result.Data == nil {
		return ""
	}
	if data, ok := result.Data.(map[string]interface{}); ok {
		if bvid, ok := data["bvid"].(string); ok && bvid != "" {
			return bvid
		}
	}
	return ""
}

// CredToSDKLogin 将 auth.LoginInfo 转换为 SDK LoginInfo
func CredToSDKLogin(cred *auth.LoginInfo) *bilibili.LoginInfo {
	sdk := &bilibili.LoginInfo{
		TokenInfo: bilibili.TokenInfo{
			AccessToken: cred.TokenInfo.AccessToken,
		},
	}
	// Build cookie info
	cookies := make([]map[string]interface{}, 0)
	for name, val := range cred.Cookies {
		cookies = append(cookies, map[string]interface{}{
			"name":  name,
			"value": val,
		})
	}
	sdk.CookieInfo = map[string]interface{}{
		"cookies": cookies,
	}
	return sdk
}

func truncate(s string, n int) string {
	if len([]rune(s)) > n {
		return string([]rune(s)[:n])
	}
	return s
}
