package bili

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/difyz9/bilibili-go-sdk/bilibili"

	"github.com/zolagz/ytb2bili-go/internal/auth"
)

const (
	// BCCItemLimit 每条 B站 字幕轨道的最大字幕条数限制
	// B站 对每轨字幕限制约 460~500 条，超过会报 code=79014
	BCCItemLimit = 450
)

type UploadParams struct {
	VideoPath string
	Title     string
	Desc      string
	Tags      []string
	Source    string
	Tid       int
	CoverPath string
}

func Upload(cred *auth.LoginInfo, params *UploadParams) (string, error) {
	// Build SDK LoginInfo
	sdkLogin := CredToSDKLogin(cred)

	// Create clients with 15-minute timeout for all API calls
	httpClient := &http.Client{Timeout: 15 * time.Minute}
	uploadClient := bilibili.NewUploadClient(sdkLogin, bilibili.WithHTTPClient(httpClient), bilibili.WithTimeout(15*time.Minute))

	// 1. Upload video file
	log.Printf("📹 步骤1: 上传视频文件")
	video, err := uploadClient.UploadVideo(params.VideoPath)
	if err != nil {
		return "", fmt.Errorf("上传视频文件失败: %w", err)
	}
	log.Printf("📹 视频上传完成: title=%s", video.Title)

	// 2. Upload cover (optional) with timeout
	log.Printf("🖼️ 步骤2: 准备封面上传 (coverPath=%q, exists=%v)", params.CoverPath, fileExists(params.CoverPath))
	coverURL := ""
	if params.CoverPath != "" {
		if _, err := os.Stat(params.CoverPath); err == nil {
			log.Printf("🖼️ 开始上传封面 (30s超时)")
			coverDone := make(chan string, 1)
			go func() {
				url, err := uploadClient.UploadCover(params.CoverPath)
				if err != nil {
					log.Printf("⚠ 封面上传失败(忽略): %v", err)
					coverDone <- ""
				} else {
					log.Printf("✅ 封面上传成功: %s", url)
					coverDone <- url
				}
			}()
			select {
			case url := <-coverDone:
				coverURL = url
				log.Printf("🖼️ 封面获取完成: %s", coverURL)
			case <-time.After(30 * time.Second):
				log.Printf("⚠ 封面上传超时(忽略)")
			}
		} else {
			log.Printf("⚠ 封面文件不存在: %v", err)
		}
	}

	// 3. Submit video
	log.Printf("📤 步骤3: 开始提交视频到B站 (title=%q)", params.Title)
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
	log.Printf("📤 SubmitVideo 调用前")

	result, err := uploadClient.SubmitVideo(studio)
	log.Printf("📤 SubmitVideo 返回: err=%v", err)
	if err != nil {
		return "", fmt.Errorf("投稿失败: %w", err)
	}

	bvid := extractBVID(result)
	if bvid == "" {
		return "", fmt.Errorf("投稿响应中未获取到BVID")
	}
	log.Printf("✅ 投稿成功: BVID=%s", bvid)

	return bvid, nil
}

// UploadSubtitle 上传单个字幕文件到已发布的B站视频
// 使用 SubtitleUploader（获取 CID → 转换 SRT → 保存草稿）
// 支持：语言回退（79011错误时尝试替代语言码）、BCC条数限制处理
func UploadSubtitle(cred *auth.LoginInfo, bvid, subtitlePath, language string) error {
	if _, err := os.Stat(subtitlePath); err != nil {
		return fmt.Errorf("字幕文件不存在: %w", err)
	}

	sdkLogin := CredToSDKLogin(cred)
	client := bilibili.NewClient()
	uploader := bilibili.NewSubtitleUploader(client, sdkLogin)

	// 1. 获取视频信息（CID）
	log.Printf("  [字幕] 获取视频 %s 的 CID...", bvid)
	videoInfo, err := uploader.GetVideoInfo(bvid)
	if err != nil {
		return fmt.Errorf("获取视频信息失败 (bvid=%s): %w", bvid, err)
	}
	log.Printf("  [字幕] CID=%d, AID=%d", videoInfo.CID, videoInfo.AID)

	// 2. 将 SRT 转换为 BCC JSON，并处理条数限制
	subtitle, err := bilibili.LoadSRTAsBCC(subtitlePath)
	if err != nil {
		return fmt.Errorf("解析字幕文件失败: %w", err)
	}

	// BCC 条数限制检查（B站每轨限制 ~460-500 条）
	if len(subtitle.Body) > BCCItemLimit {
		log.Printf("  ⚠ [字幕] 字幕条数 %d 超过限制 %d，将截断至 %d 条",
			len(subtitle.Body), BCCItemLimit, BCCItemLimit)
		subtitle.Body = subtitle.Body[:BCCItemLimit]
	}

	// 3. 规范化语言码
	lang := bilibili.NormalizeSubtitleLanguage(language)

	// 4. 尝试上传，支持语言回退
	err = uploader.SaveSubtitleDraft(bvid, videoInfo.CID, subtitle, lang)
	if err != nil {
		// 检查是否为不合法的语言错误（code=79011）
		errStr := err.Error()
		if strings.Contains(errStr, "79011") && lang != "zh" {
			// 回退到 zh
			log.Printf("  ⚠ [字幕] 语言 %q 被拒绝 (79011)，回退到 zh", lang)
			err = uploader.SaveSubtitleDraft(bvid, videoInfo.CID, subtitle, "zh")
			if err != nil {
				return fmt.Errorf("上传字幕失败 (bvid=%s, lang=%s→zh): %w", bvid, lang, err)
			}
			return nil
		}
		return fmt.Errorf("上传字幕失败 (bvid=%s, lang=%s): %w", bvid, lang, err)
	}

	return nil
}

// CheckReviewStatus 检查B站视频审核状态
func CheckReviewStatus(cred *auth.LoginInfo, bvid string) (*bilibili.VideoReviewStatus, error) {
	client := bilibili.NewClient()
	cookies := BuildCookiesString(cred)
	return client.GetVideoReviewStatus(bvid, cookies)
}

// WaitForReviewPassed 等待B站视频审核通过（每3分钟轮询，最长等待24小时）
func WaitForReviewPassed(cred *auth.LoginInfo, bvid string) (*bilibili.VideoReviewStatus, error) {
	client := bilibili.NewClient()
	cookies := BuildCookiesString(cred)
	return client.WaitForVideoReviewPassed(bvid, cookies, 3*time.Minute, 24*time.Hour)
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
	// 使用 []interface{} 确保 SDK 的 GetCookieString() 能正确类型断言
	cookies := make([]interface{}, 0)
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

// BuildCookiesString 从 LoginInfo 构建 Cookie 字符串
func BuildCookiesString(cred *auth.LoginInfo) string {
	var parts []string
	for name, val := range cred.Cookies {
		parts = append(parts, name+"="+val)
	}
	return strings.Join(parts, "; ")
}

func truncate(s string, n int) string {
	if len([]rune(s)) > n {
		return string([]rune(s)[:n])
	}
	return s
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
