package cli

// // B站账号: login/whoami/accounts（多账号凭证管理）

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/zolagz/ytb2bili-go/internal/auth"
	"github.com/zolagz/ytb2bili-go/internal/cdp"
	"github.com/zolagz/ytb2bili-go/internal/storage"
)

// ─── Login ─────────────────────────────────────────────────────────────────

func newLoginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "B站扫码登录（--account 指定账号名支持多账号）",
		RunE: func(cmd *cobra.Command, args []string) error {
			acctName, _ := cmd.Flags().GetString("account")
			cfg := loadConfig()
			credDir := filepath.Join(cfg.DataDir, "cookies")
			store := storage.NewCredentialStore(credDir)

			if acctName != "" {
				fmt.Printf("👤 登录账号: %s\n", acctName)
			}

			fmt.Println("📱 获取二维码...")
			qr, err := auth.GetQRCode()
			if err != nil {
				return err
			}

			qrPath := filepath.Join(credDir, "bilibili_qrcode.png")
			SaveQRCode(qr.URL, qrPath)
			PrintQRCodeTerminal(qr.URL)

			// 后台打开浏览器
			go func() {
				cm := cdp.NewChromeManager(
					cdp.WithPort(9222),
					cdp.WithUserDataDir(filepath.Join(cfg.DataDir, "browser_data")),
				)
				cdp.RegisterCleanup(cm)
				if ctx, cancel, err := cm.Connect(); err == nil {
					defer cancel()
					if err := cdp.OpenURL(ctx, qr.URL); err == nil {
						fmt.Fprintf(os.Stderr, "🌐 已在 Chrome 中打开扫码页面\n")
						return
					}
				}
				if err := cdp.OpenURLSystem(qr.URL); err == nil {
					fmt.Fprintf(os.Stderr, "🌐 已在浏览器中打开二维码页面\n")
					return
				}
				fmt.Fprintf(os.Stderr, "💡 请手动打开二维码图片: %s\n", qrPath)
			}()

			fmt.Fprintf(os.Stderr, "⏳ 等待扫码...（最长120秒）\n")

			pollCtx, pollCancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer pollCancel()

			cred, err := auth.PollQRCodeContext(pollCtx, qr.AuthCode, 120*time.Second)
			if err != nil {
				return fmt.Errorf("登录失败: %w", err)
			}

			store.Save(cred, acctName)
			fmt.Println("\n✅ ===== 扫码成功! =====")
			if acctName != "" {
				fmt.Printf("   账号: %s\n", acctName)
			}
			fmt.Println("   登录凭据已保存")

			info, _ := auth.GetUserInfo(cred)
			if name, ok := info["name"].(string); ok {
				mid, _ := info["mid"].(float64)
				fmt.Printf("   用户: %s (UID: %.0f)\n", name, mid)
				if cred.TokenInfo.Uname == "" || cred.TokenInfo.Uname != name {
					cred.TokenInfo.Uname = name
				}
				if cred.TokenInfo.Mid == 0 && mid > 0 {
					cred.TokenInfo.Mid = int64(mid)
				}
				store.Save(cred, acctName)
			}
			fmt.Println("✅ =====================")
			return nil
		},
	}
	cmd.Flags().String("account", "", "账号名（登录为多账号，投稿时按类型路由）")
	return cmd
}

// ─── WhoAmI ────────────────────────────────────────────────────────────────

// ─── WhoAmI ────────────────────────────────────────────────────────────────

func newWhoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "查看当前登录的B站账号信息",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			credDir := filepath.Join(cfg.DataDir, "cookies")
			cs := storage.NewCredentialStore(credDir)

			if !cs.Exists() {
				fmt.Println("❌ 未登录")
				fmt.Println("💡 请先执行: ytb login")
				return nil
			}

			var cred auth.LoginInfo
			if err := cs.Load(&cred); err != nil {
				return fmt.Errorf("读取登录凭据失败: %w", err)
			}

			valid, err := auth.ValidateLogin(&cred)
			if err != nil || !valid {
				fmt.Println("❌ 登录已过期，请重新登录")
				fmt.Println("💡 执行: ytb login")
				return nil
			}

			fmt.Println("✅ 登录状态有效")
			if cred.TokenInfo.Uname != "" {
				fmt.Printf("   用户名: %s\n", cred.TokenInfo.Uname)
			}
			if cred.TokenInfo.Mid > 0 {
				fmt.Printf("   UID: %d\n", cred.TokenInfo.Mid)
			}
			return nil
		},
	}
}

// ─── Accounts ──────────────────────────────────────────────────────────────

// ─── Accounts ──────────────────────────────────────────────────────────────

func newAccountsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "accounts",
		Short: "列出所有已登录的B站账号",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			cs := storage.NewCredentialStore(filepath.Join(cfg.DataDir, "cookies"))
			names := cs.ListAccounts()

			if len(names) == 0 {
				fmt.Println("📭 没有多账号登录记录")
				if cs.Exists() {
					fmt.Println("   已使用默认账号（bilibili.json）")
					fmt.Println("💡 多账号登录: ytb login --account <账号名>")
				} else {
					fmt.Println("❌ 未登录任何账号")
					fmt.Println("💡 请先执行: ytb login")
				}
				return nil
			}

			fmt.Println("👥 已登录账号:")
			for _, n := range names {
				var cred auth.LoginInfo
				if err := cs.Load(&cred, n); err == nil {
					valid, _ := auth.ValidateLogin(&cred)
					status := "✅"
					if !valid {
						status = "⚠️ 过期"
					}
					uname := cred.TokenInfo.Uname
					if uname == "" {
						uname = "(未获取用户名)"
					}
					fmt.Printf("  %s %-15s %s\n", status, n, uname)
				}
			}
			return nil
		},
	}
}

// ─── Download ──────────────────────────────────────────────────────────────
