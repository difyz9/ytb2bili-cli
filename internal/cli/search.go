package cli

// // 搜索与历史：search（含 --submit）、search --history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zolagz/ytb2bili-go/internal/config"
	"github.com/zolagz/ytb2bili-go/internal/search"
	"github.com/zolagz/ytb2bili-go/internal/storage"
)

// ─── Search ────────────────────────────────────────────────────────────────

func newSearchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "搜索 YouTube 视频",
		Long: `搜索 YouTube 视频，支持过滤器和输出格式。

示例:
  ytb search "Flutter tutorial"
  ytb search --sort view_count --duration long "AI tutorial"
  ytb search --json --max 5 "Go programming"
  ytb search --submit 1 "Flutter tutorial"    # 直接提交第 1 个结果
  ytb search --history                        # 查看已提交历史`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()

			if history, _ := cmd.Flags().GetBool("history"); history {
				return runSearchHistory(cfg)
			}
			if len(args) == 0 {
				return fmt.Errorf("请输入搜索关键词")
			}

			query := strings.Join(args, " ")
			maxResults, _ := cmd.Flags().GetInt("max")
			if maxResults <= 0 {
				maxResults = 10
			}
			sortBy, _ := cmd.Flags().GetString("sort")
			uploadDate, _ := cmd.Flags().GetString("upload-date")
			duration, _ := cmd.Flags().GetString("duration")
			asJSON, _ := cmd.Flags().GetBool("json")

			searcher := search.New(maxResults)
			result, err := searcher.SearchPaginated(query, searchFilter(sortBy, uploadDate, duration), "")
			if err != nil {
				return fmt.Errorf("搜索失败: %w", err)
			}

			if asJSON {
				return printSearchJSON(result)
			}

			fmt.Print(formatSearchResults(result, submittedSet(cfg)))

			submitN, _ := cmd.Flags().GetInt("submit")
			if submitN > 0 {
				if submitN > len(result.Videos) {
					return fmt.Errorf("--submit %d 超出结果数 %d", submitN, len(result.Videos))
				}
				v := result.Videos[submitN-1]
				fmt.Printf("\n🚀 提交第 %d 个结果: %s\n", submitN, v.Title)
				_, err := processSingle(cfg, v.URL, false)
				return err
			}
			return nil
		},
	}
	cmd.Flags().Int("max", 10, "最大结果数")
	cmd.Flags().String("sort", "", "排序: relevance / upload_date / view_count / rating")
	cmd.Flags().String("upload-date", "", "上传时间: last_hour / today / this_week / this_month / this_year")
	cmd.Flags().String("duration", "", "时长: short(<4m) / medium(4-20m) / long(>20m)")
	cmd.Flags().Bool("json", false, "以 JSON 输出搜索结果")
	cmd.Flags().Bool("history", false, "查看已提交历史（无需关键词）")
	cmd.Flags().Int("submit", 0, "直接提交第 N 个结果")
	return cmd
}

// newHistoryCmd 查看已提交的投稿历史。
func newHistoryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history",
		Short: "查看已提交的投稿历史",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			history := storage.NewHistoryStore(filepath.Join(cfg.DataDir, "history"))
			videos, err := history.List()
			if err != nil {
				if os.IsNotExist(err) {
					videos = []storage.SubmittedVideo{}
				} else {
					return err
				}
			}
			if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
				if videos == nil {
					videos = []storage.SubmittedVideo{}
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(videos)
			}
			fmt.Print(renderHistory(videos))
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "以 JSON 输出")
	return cmd
}

// runSearchHistory 查看提交历史
func runSearchHistory(cfg *config.Config) error {
	history := storage.NewHistoryStore(filepath.Join(cfg.DataDir, "history"))
	videos, err := history.List()
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("📭 暂无提交历史")
			return nil
		}
		return err
	}
	fmt.Print(renderHistory(videos))
	return nil
}

// submittedSet 返回所有已提交的 YouTube ID 集合
func submittedSet(cfg *config.Config) map[string]bool {
	history := storage.NewHistoryStore(filepath.Join(cfg.DataDir, "history"))
	videos, err := history.List()
	if err != nil {
		return nil
	}
	set := make(map[string]bool, len(videos))
	for _, v := range videos {
		set[v.YouTubeID] = true
	}
	return set
}

// printSearchJSON 以 JSON 输出搜索结果
func printSearchJSON(result *search.SearchResult) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

// ─── Subtitle ──────────────────────────────────────────────────────────────
