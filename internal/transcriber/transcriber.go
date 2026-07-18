package transcriber

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	apiBase        = "https://member.bilibili.com/x/bcut/rubick-interface"
	apiReqUpload   = apiBase + "/resource/create"
	apiCommit      = apiBase + "/resource/create/complete"
	apiCreateTask  = apiBase + "/task"
	apiQueryResult = apiBase + "/task/result"
	modelID        = "8"
)

// 上传响应
type uploadResponse struct {
	Code int `json:"code"`
	Data struct {
		InBossKey  string   `json:"in_boss_key"`
		ResourceID string   `json:"resource_id"`
		UploadID   string   `json:"upload_id"`
		UploadURLs []string `json:"upload_urls"`
		PerSize    int      `json:"per_size"`
		Size       int      `json:"size"`
	} `json:"data"`
}

// 提交响应
type commitResponse struct {
	Code int `json:"code"`
	Data struct {
		DownloadURL string `json:"download_url"`
	} `json:"data"`
}

// 任务响应
type taskResponse struct {
	Code int `json:"code"`
	Data struct {
		TaskID string `json:"task_id"`
	} `json:"data"`
}

// 查询响应
type queryResponse struct {
	Code int `json:"code"`
	Data struct {
		State  int    `json:"state"`
		Result string `json:"result"`
	} `json:"data"`
}

// BCut 结果
type bcutResult struct {
	Language   string `json:"language"`
	Utterances []struct {
		Transcript string  `json:"transcript"`
		StartTime  float64 `json:"start_time"`
		EndTime    float64 `json:"end_time"`
	} `json:"utterances"`
}

func BcutASR(videoPath, outputDir, videoID string) (string, error) {
	return BcutASRContext(context.Background(), videoPath, outputDir, videoID)
}

func BcutASRContext(ctx context.Context, videoPath, outputDir, videoID string) (string, error) {

	// Step 1: Extract audio
	audioPath := filepath.Join(outputDir, "audio.mp3")
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", videoPath,
		"-vn", "-acodec", "libmp3lame", "-ab", "128k",
		"-ar", "16000", "-ac", "1", audioPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("提取音频失败: %s", string(out))
	}
	defer os.Remove(audioPath)

	// Read file
	fileData, err := os.ReadFile(audioPath)
	if err != nil {
		return "", fmt.Errorf("读取音频失败: %w", err)
	}
	fmt.Printf("  音频大小: %d KB\n", len(fileData)/1024)

	// Step 2: Request upload
	fmt.Print("  申请上传... ")
	uploadResp, err := requestUpload(ctx, fileData)
	if err != nil {
		return "", fmt.Errorf("申请上传失败: %w", err)
	}
	fmt.Printf("OK (%d 个分片)\n", len(uploadResp.Data.UploadURLs))

	// Step 3: Upload parts
	fmt.Print("  上传分片... ")
	etags, err := uploadParts(ctx, fileData, uploadResp)
	if err != nil {
		return "", fmt.Errorf("上传分片失败: %w", err)
	}
	fmt.Println("OK")

	// Step 4: Commit upload
	fmt.Print("  提交上传... ")
	downloadURL, err := commitUpload(ctx, uploadResp, etags)
	if err != nil {
		return "", fmt.Errorf("提交上传失败: %w", err)
	}
	fmt.Println("OK")

	// Step 5: Create task
	fmt.Print("  创建转录任务... ")
	taskID, err := createTask(ctx, downloadURL)
	if err != nil {
		return "", fmt.Errorf("创建任务失败: %w", err)
	}
	fmt.Printf("OK (task_id: %s)\n", taskID[:min(16, len(taskID))])

	// Step 6: Query result
	fmt.Print("  等待转录结果... ")
	result, err := queryResult(ctx, taskID)
	if err != nil {
		return "", fmt.Errorf("查询结果失败: %w", err)
	}
	fmt.Printf("OK (%d 条字幕)\n", len(result.Utterances))

	// Step 7: Generate SRT — use videoID as filename for BuildSubtitleCandidates compatibility
	if videoID == "" {
		videoID = "subtitle"
	}
	srtPath := filepath.Join(outputDir, videoID+".srt")
	if err := generateSRT(result, srtPath); err != nil {
		return "", err
	}
	return srtPath, nil
}

func requestUpload(ctx context.Context, fileData []byte) (*uploadResponse, error) {
	payload := map[string]interface{}{
		"type":             2,
		"name":             "audio.mp3",
		"size":             len(fileData),
		"ResourceFileType": "mp3",
		"model_id":         modelID,
	}
	payloadBytes, _ := json.Marshal(payload)

	req, _ := http.NewRequestWithContext(ctx, "POST", apiReqUpload, bytes.NewReader(payloadBytes))
	req.Header.Set("User-Agent", "Bilibili/1.0.0 (https://www.bilibili.com)")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result uploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("code=%d", result.Code)
	}
	return &result, nil
}

func uploadParts(ctx context.Context, fileData []byte, uploadResp *uploadResponse) ([]string, error) {
	perSize := uploadResp.Data.PerSize
	if perSize <= 0 {
		perSize = 5 * 1024 * 1024
	}
	totalParts := len(uploadResp.Data.UploadURLs)
	etags := make([]string, 0, totalParts)

	for i := 0; i < totalParts; i++ {
		start := i * perSize
		end := (i + 1) * perSize
		if end > len(fileData) {
			end = len(fileData)
		}

		etag, err := uploadSinglePart(ctx, uploadResp.Data.UploadURLs[i], fileData[start:end])
		if err != nil {
			return nil, fmt.Errorf("分片 %d 上传失败: %w", i+1, err)
		}
		etags = append(etags, etag)
	}
	return etags, nil
}

func uploadSinglePart(ctx context.Context, uploadURL string, chunk []byte) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, "PUT", uploadURL, bytes.NewReader(chunk))
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return strings.Trim(resp.Header.Get("Etag"), "\""), nil
}

func commitUpload(ctx context.Context, uploadResp *uploadResponse, etags []string) (string, error) {
	payload := map[string]interface{}{
		"InBossKey":  uploadResp.Data.InBossKey,
		"ResourceId": uploadResp.Data.ResourceID,
		"Etags":      strings.Join(etags, ","),
		"UploadId":   uploadResp.Data.UploadID,
		"model_id":   modelID,
	}
	payloadBytes, _ := json.Marshal(payload)

	req, _ := http.NewRequestWithContext(ctx, "POST", apiCommit, bytes.NewReader(payloadBytes))
	req.Header.Set("User-Agent", "Bilibili/1.0.0 (https://www.bilibili.com)")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result commitResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Code != 0 {
		return "", fmt.Errorf("code=%d", result.Code)
	}
	return result.Data.DownloadURL, nil
}

func createTask(ctx context.Context, downloadURL string) (string, error) {
	payload := map[string]interface{}{
		"resource": downloadURL,
		"model_id": modelID,
	}
	payloadBytes, _ := json.Marshal(payload)

	req, _ := http.NewRequestWithContext(ctx, "POST", apiCreateTask, bytes.NewReader(payloadBytes))
	req.Header.Set("User-Agent", "Bilibili/1.0.0 (https://www.bilibili.com)")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result taskResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Code != 0 {
		return "", fmt.Errorf("code=%d", result.Code)
	}
	return result.Data.TaskID, nil
}

func queryResult(ctx context.Context, taskID string) (*bcutResult, error) {
	maxRetries := 300
	for i := 0; i < maxRetries; i++ {
		req, _ := http.NewRequestWithContext(ctx, "GET", apiQueryResult, nil)
		q := req.URL.Query()
		q.Add("model_id", modelID)
		q.Add("task_id", taskID)
		req.URL.RawQuery = q.Encode()
		req.Header.Set("User-Agent", "Bilibili/1.0.0 (https://www.bilibili.com)")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var queryResp queryResponse
		if err := json.Unmarshal(bodyBytes, &queryResp); err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		// State 4 = success
		if queryResp.Data.State == 4 {
			var result bcutResult
			if err := json.Unmarshal([]byte(queryResp.Data.Result), &result); err != nil {
				return nil, fmt.Errorf("解析结果失败: %w", err)
			}
			return &result, nil
		}
		// State 3 = failed
		if queryResp.Data.State == 3 {
			return nil, fmt.Errorf("转写任务失败 (state=%d)", queryResp.Data.State)
		}

		if i%10 == 0 && i > 0 {
			fmt.Printf("  等待中... (state=%d, %ds)\n", queryResp.Data.State, i*2)
		}
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("转写超时")
}

func generateSRT(r *bcutResult, path string) error {
	var lines []string
	for i, u := range r.Utterances {
		from := formatSRTTime(u.StartTime / 1000.0)
		to := formatSRTTime(u.EndTime / 1000.0)
		text := strings.TrimSpace(u.Transcript)
		if text == "" {
			text = " "
		}
		lines = append(lines,
			strconv.Itoa(i+1),
			fmt.Sprintf("%s --> %s", from, to),
			text,
			"",
		)
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

func formatSRTTime(seconds float64) string {
	h := int(seconds) / 3600
	m := (int(seconds) % 3600) / 60
	s := int(seconds) % 60
	ms := int((seconds - float64(int(seconds))) * 1000)
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, ms)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
