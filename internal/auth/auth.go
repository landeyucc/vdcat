package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type Authenticator struct {
	user      string
	pass      string
	staticTok string
	secret    []byte
	enabled   bool

	mu       sync.RWMutex
	sessions map[string]int64
	ttl      time.Duration
}

func New(user, pass, staticTok string) *Authenticator {
	enabled := pass != "" || staticTok != ""
	secret := []byte(os.Getenv("VDCAT_SECRET"))
	if len(secret) == 0 {
		b := make([]byte, 32)
		_, _ = rand.Read(b)
		secret = b
	}
	return &Authenticator{
		user:      orDefault(user, "admin"),
		pass:      pass,
		staticTok: staticTok,
		secret:    secret,
		enabled:   enabled,
		sessions:  map[string]int64{},
		ttl:       24 * 7 * time.Hour,
	}
}

func orDefault(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func (a *Authenticator) Enabled() bool { return a.enabled }

type claims struct {
	User string `json:"u"`
	Exp  int64  `json:"exp"`
}

// Login 校验用户名密码，成功返回签名 token
func (a *Authenticator) Login(user, pass string) (string, error) {
	if !a.enabled {
		return "", fmt.Errorf("未启用登录")
	}
	if subtle.ConstantTimeCompare([]byte(user), []byte(a.user)) != 1 ||
		subtle.ConstantTimeCompare([]byte(pass), []byte(a.pass)) != 1 {
		return "", fmt.Errorf("用户名或密码错误")
	}
	return a.issueToken(user), nil
}

func (a *Authenticator) issueToken(user string) string {
	body := claims{User: user, Exp: time.Now().Add(a.ttl).Unix()}
	raw, _ := json.Marshal(body)
	payload := b64(raw)
	sig := a.sign(payload)
	tok := payload + "." + sig

	a.mu.Lock()
	a.sessions[tok] = body.Exp
	a.mu.Unlock()
	return tok
}

func (a *Authenticator) sign(payload string) string {
	mac := hmac.New(sha256.New, a.secret)
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

// Verify 校验 Bearer token
func (a *Authenticator) Verify(tok string) bool {
	if !a.enabled {
		return true
	}
	if tok == "" {
		return false
	}
	if a.staticTok != "" && subtle.ConstantTimeCompare([]byte(tok), []byte(a.staticTok)) == 1 {
		return true
	}

	a.mu.RLock()
	exp, ok := a.sessions[tok]
	a.mu.RUnlock()
	if ok {
		return time.Now().Unix() < exp
	}

	parts := strings.SplitN(tok, ".", 2)
	if len(parts) != 2 {
		return false
	}
	if subtle.ConstantTimeCompare([]byte(a.sign(parts[0])), []byte(parts[1])) != 1 {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	var c claims
	if json.Unmarshal(raw, &c) != nil {
		return false
	}
	return time.Now().Unix() < c.Exp
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// Middleware 保护 /api（登录、健康检查、媒体代理、外部开放接口除外）
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		if !a.enabled ||
			r.URL.Path == "/api/login" ||
			r.URL.Path == "/api/health" ||
			r.URL.Path == "/api/media" ||
			strings.HasPrefix(r.URL.Path, "/api/external/") {
			next.ServeHTTP(w, r)
			return
		}
		tok := extractToken(r)
		if !a.Verify(tok) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"ok":false,"error":"未授权或登录已过期"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func extractToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	// 兼容 query token（便于文件直链/下载场景）
	return strings.TrimSpace(r.URL.Query().Get("token"))
}
