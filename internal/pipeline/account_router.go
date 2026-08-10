package pipeline

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zolagz/ytb2bili-go/internal/auth"
	"github.com/zolagz/ytb2bili-go/internal/config"
	"github.com/zolagz/ytb2bili-go/internal/storage"
)

// accountRouter 多 B站账号路由：根据稿件标题/标签匹配账号规则，决定投稿到哪个账号。
type accountRouter struct {
	config *config.Config
}

// resolve 根据稿件信息（标题+标签）选出投稿账号名（空=默认账号）。
// 路由规则（config.yaml accounts 段）：
//   - 命中任意账号的 TypeRule 关键词 → 使用该账号
//   - 无命中 → 使用 IsDefault 账号；无默认账号 → 空（旧版单账号）
func (r *accountRouter) resolve(title string, tags []string) string {
	cfg := r.config
	if cfg == nil || len(cfg.Accounts) == 0 {
		return ""
	}
	// 稿件匹配文本：标题 + 标签
	haystack := strings.ToLower(title)
	for _, t := range tags {
		haystack += " " + strings.ToLower(t)
	}
	// 第一遍：精确规则匹配（先匹配非默认账号的规则）
	for _, acct := range cfg.Accounts {
		if acct.Name == "" || acct.IsDefault {
			continue
		}
		if matchRule(haystack, acct.TypeRule) {
			return acct.Name
		}
	}
	// 第二遍：默认账号
	for _, acct := range cfg.Accounts {
		if acct.IsDefault && acct.Name != "" {
			return acct.Name
		}
	}
	return ""
}

// matchRule 判断 haystack 是否命中任意关键词（子串匹配，忽略大小写）
func matchRule(haystack string, rules []string) bool {
	for _, rule := range rules {
		rule = strings.TrimSpace(strings.ToLower(rule))
		if rule == "" {
			continue
		}
		if strings.Contains(haystack, rule) {
			return true
		}
	}
	return false
}

// loadCredentialFor 加载指定账号（或默认）的凭证
func (r *accountRouter) loadCredentialFor(accountName string) (*auth.LoginInfo, error) {
	cs := storage.NewCredentialStore(filepath.Join(r.config.DataDir, "cookies"))
	var cred auth.LoginInfo
	if err := cs.Load(&cred, accountName); err != nil {
		if accountName == "" {
			return nil, fmt.Errorf("请先登录: ytb2bili login")
		}
		return nil, fmt.Errorf("账号 %q 未登录: ytb2bili login --account %q", accountName, accountName)
	}
	return &cred, nil
}

// pickAccount 路由 + 加载凭证一步到位
// title/tags 用于路由；accountOverride 为显式指定账号（publish --account）
func (r *accountRouter) pickAccount(title string, tags []string, accountOverride string) (*auth.LoginInfo, string, error) {
	acct := accountOverride
	if acct == "" {
		acct = r.resolve(title, tags)
	}
	cred, err := r.loadCredentialFor(acct)
	if err != nil {
		return nil, "", err
	}
	if acct == "" && cred.TokenInfo.Uname != "" {
		acct = cred.TokenInfo.Uname
	}
	return cred, acct, nil
}
