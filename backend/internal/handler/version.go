package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// 可由编译 ldflags 注入：-X github.com/tuzi/cdk-recharge-system/internal/handler.BuildVersion=x
var BuildVersion = ""

// 默认仓库：白标 CDK 门户公开仓
const defaultGitHubRepo = "spacexcard/spacex_card_cdk_auto"

var (
	versionCacheMu   sync.Mutex
	versionCacheAt   time.Time
	versionCacheBody gin.H
	versionCacheTTL  = 5 * time.Minute
)

func resolveLocalVersion() string {
	if v := strings.TrimSpace(BuildVersion); v != "" {
		return strings.TrimPrefix(v, "v")
	}
	if v := strings.TrimSpace(os.Getenv("CDK_APP_VERSION")); v != "" {
		return strings.TrimPrefix(v, "v")
	}
	// 尝试从工作目录 / 可执行文件旁的 VERSION 读取
	candidates := []string{"VERSION", filepath.Join("..", "VERSION"), filepath.Join("..", "..", "VERSION")}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "VERSION"))
	}
	for _, p := range candidates {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		line := strings.TrimSpace(strings.SplitN(string(b), "\n", 2)[0])
		if line != "" {
			return strings.TrimPrefix(line, "v")
		}
	}
	return "0.0.0-dev"
}

func githubRepo() string {
	if r := strings.TrimSpace(os.Getenv("CDK_GITHUB_REPO")); r != "" {
		return strings.Trim(r, "/")
	}
	return defaultGitHubRepo
}

// AdminSystemVersion GET /api/v1/admin/system/version
// 返回本机版本 + GitHub 最新 release/tag，并标明是否有更新。
func AdminSystemVersion(c *gin.Context) {
	force := c.Query("refresh") == "1" || c.Query("force") == "1"
	local := resolveLocalVersion()
	repo := githubRepo()

	versionCacheMu.Lock()
	if !force && versionCacheBody != nil && time.Since(versionCacheAt) < versionCacheTTL {
		body := cloneH(versionCacheBody)
		versionCacheMu.Unlock()
		body["current"] = local
		body["update_available"] = isRemoteNewer(local, strField(body, "latest"))
		c.JSON(http.StatusOK, body)
		return
	}
	versionCacheMu.Unlock()

	latest, releaseURL, tagURL, source, errMsg := fetchGitHubLatest(repo)
	update := isRemoteNewer(local, latest)
	body := gin.H{
		"current":          local,
		"latest":           latest,
		"update_available": update,
		"github_repo":      repo,
		"release_url":      releaseURL,
		"tags_url":         tagURL,
		"source":           source,
		"checked_at":       time.Now().UTC().Format(time.RFC3339),
	}
	if errMsg != "" {
		body["github_error"] = errMsg
	}

	versionCacheMu.Lock()
	versionCacheBody = cloneH(body)
	versionCacheAt = time.Now()
	versionCacheMu.Unlock()

	c.JSON(http.StatusOK, body)
}

func cloneH(in gin.H) gin.H {
	out := gin.H{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

func strField(h gin.H, k string) string {
	if h == nil {
		return ""
	}
	if s, ok := h[k].(string); ok {
		return s
	}
	return ""
}

func fetchGitHubLatest(repo string) (latest, releaseURL, tagsURL, source, errMsg string) {
	tagsURL = "https://github.com/" + repo + "/tags"
	releaseURL = "https://github.com/" + repo + "/releases"
	client := &http.Client{Timeout: 8 * time.Second}

	// 1) releases/latest
	req, _ := http.NewRequest(http.MethodGet, "https://api.github.com/repos/"+repo+"/releases/latest", nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "cdk-recharge-system-version-check")
	if tok := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	if resp, err := client.Do(req); err == nil {
		defer resp.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if resp.StatusCode == 200 {
			var payload struct {
				TagName string `json:"tag_name"`
				HTMLURL string `json:"html_url"`
			}
			if json.Unmarshal(raw, &payload) == nil && strings.TrimSpace(payload.TagName) != "" {
				latest = strings.TrimPrefix(strings.TrimSpace(payload.TagName), "v")
				if payload.HTMLURL != "" {
					releaseURL = payload.HTMLURL
				}
				source = "release"
				return
			}
		}
		if resp.StatusCode != 404 {
			errMsg = "releases HTTP " + resp.Status
		}
	} else {
		errMsg = err.Error()
	}

	// 2) fallback: tags
	req2, _ := http.NewRequest(http.MethodGet, "https://api.github.com/repos/"+repo+"/tags?per_page=1", nil)
	req2.Header.Set("Accept", "application/vnd.github+json")
	req2.Header.Set("User-Agent", "cdk-recharge-system-version-check")
	if tok := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); tok != "" {
		req2.Header.Set("Authorization", "Bearer "+tok)
	}
	if resp, err := client.Do(req2); err == nil {
		defer resp.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if resp.StatusCode == 200 {
			var tags []struct {
				Name string `json:"name"`
			}
			if json.Unmarshal(raw, &tags) == nil && len(tags) > 0 && strings.TrimSpace(tags[0].Name) != "" {
				latest = strings.TrimPrefix(strings.TrimSpace(tags[0].Name), "v")
				source = "tag"
				return
			}
		} else if errMsg == "" {
			errMsg = "tags HTTP " + resp.Status
		}
	} else if errMsg == "" {
		errMsg = err.Error()
	}
	return
}

// 简单 semver 比较：a < b 时认为远程更新。非数字段按字典序。
func isRemoteNewer(current, latest string) bool {
	c := normalizeVer(current)
	l := normalizeVer(latest)
	if c == "" || l == "" || c == l {
		return false
	}
	cp := strings.Split(c, ".")
	lp := strings.Split(l, ".")
	n := len(cp)
	if len(lp) > n {
		n = len(lp)
	}
	for i := 0; i < n; i++ {
		var cv, lv int
		if i < len(cp) {
			cv = atoiLeading(cp[i])
		}
		if i < len(lp) {
			lv = atoiLeading(lp[i])
		}
		if lv > cv {
			return true
		}
		if lv < cv {
			return false
		}
	}
	return false
}

func normalizeVer(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	v = strings.TrimPrefix(v, "v")
	// 去掉 -dev / +meta
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	return v
}

func atoiLeading(s string) int {
	n := 0
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			break
		}
		n = n*10 + int(ch-'0')
	}
	return n
}
