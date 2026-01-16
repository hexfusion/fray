package oci

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// RegistryAuth reads credentials from container config files.
type RegistryAuth struct {
	mu       sync.RWMutex
	tokens   map[string]tokenEntry
	insecure map[string]bool
}

type tokenEntry struct {
	token   string
	expires time.Time
}

type authConfig struct {
	Auths map[string]struct {
		Auth string `json:"auth"`
	} `json:"auths"`
}

type challenge struct {
	realm   string
	service string
	scope   string
}

// NewRegistryAuth creates an auth provider that reads container credentials.
func NewRegistryAuth() *RegistryAuth {
	return &RegistryAuth{
		tokens:   make(map[string]tokenEntry, 8),
		insecure: make(map[string]bool),
	}
}

// SetInsecure marks a registry as insecure (HTTP instead of HTTPS).
func (r *RegistryAuth) SetInsecure(registry string, insecure bool) {
	r.insecure[registry] = insecure
}

func (r *RegistryAuth) registryURL(registry string) string {
	scheme := "https"
	if r.insecure[registry] {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s", scheme, registry)
}

// GetAuth returns the authorization header for a registry and repo.
func (r *RegistryAuth) GetAuth(ctx context.Context, registry, repo string) (string, error) {
	cacheKey := registry + "/" + repo

	r.mu.RLock()
	if entry, ok := r.tokens[cacheKey]; ok && time.Now().Before(entry.expires) {
		r.mu.RUnlock()
		return "Bearer " + entry.token, nil
	}
	r.mu.RUnlock()

	username, password := r.loadCredentials(registry)

	ch, err := r.fetchChallenge(ctx, registry)
	if err == nil && ch.realm != "" {
		token, err := r.getToken(ctx, ch, repo, username, password)
		if err != nil {
			return "", err
		}

		r.mu.Lock()
		r.tokens[cacheKey] = tokenEntry{
			token:   token,
			expires: time.Now().Add(5 * time.Minute),
		}
		r.mu.Unlock()

		return "Bearer " + token, nil
	}

	if username != "" && password != "" {
		auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
		return "Basic " + auth, nil
	}

	return "", nil
}

func (r *RegistryAuth) fetchChallenge(ctx context.Context, registry string) (*challenge, error) {
	url := fmt.Sprintf("%s/v2/", r.registryURL(registry))

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode == http.StatusUnauthorized {
		return parseChallenge(resp.Header.Get("WWW-Authenticate")), nil
	}

	return nil, nil
}

func parseChallenge(header string) *challenge {
	if header == "" {
		return nil
	}

	header = strings.TrimPrefix(header, "Bearer ")
	ch := &challenge{}

	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if k, v, ok := strings.Cut(part, "="); ok {
			v = strings.Trim(v, "\"")
			switch k {
			case "realm":
				ch.realm = v
			case "service":
				ch.service = v
			case "scope":
				ch.scope = v
			}
		}
	}

	return ch
}

func (r *RegistryAuth) loadCredentials(registry string) (string, string) {
	configPaths := make([]string, 0, 4)

	if xdgRuntime := os.Getenv("XDG_RUNTIME_DIR"); xdgRuntime != "" {
		configPaths = append(configPaths, filepath.Join(xdgRuntime, "containers/auth.json"))
	}

	if home, err := os.UserHomeDir(); err == nil {
		configPaths = append(configPaths,
			filepath.Join(home, ".docker/config.json"),
			filepath.Join(home, ".config/containers/auth.json"),
		)
	}

	configPaths = append(configPaths, "/etc/containers/auth.json")

	for _, path := range configPaths {
		username, password, err := r.loadFromFile(path, registry)
		if err == nil && username != "" {
			return username, password
		}
	}

	return "", ""
}

func (r *RegistryAuth) loadFromFile(path, registry string) (string, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}

	var config authConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return "", "", err
	}

	if auth, ok := config.Auths[registry]; ok && auth.Auth != "" {
		return decodeAuth(auth.Auth)
	}

	if auth, ok := config.Auths["https://"+registry]; ok && auth.Auth != "" {
		return decodeAuth(auth.Auth)
	}

	if registry == DockerHubRegistry {
		for _, key := range []string{DockerHubAlias, "https://index.docker.io/v1/", "index.docker.io"} {
			if auth, ok := config.Auths[key]; ok && auth.Auth != "" {
				return decodeAuth(auth.Auth)
			}
		}
	}

	return "", "", nil
}

func decodeAuth(encoded string) (string, string, error) {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", "", err
	}

	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid auth format")
	}

	return parts[0], parts[1], nil
}

func (r *RegistryAuth) getToken(ctx context.Context, ch *challenge, repo, username, password string) (string, error) {
	u, err := url.Parse(ch.realm)
	if err != nil {
		return "", err
	}

	q := u.Query()
	if ch.service != "" {
		q.Set("service", ch.service)
	}
	q.Set("scope", fmt.Sprintf("repository:%s:pull", repo))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return "", err
	}

	if username != "" && password != "" {
		req.SetBasicAuth(username, password)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request failed: %s", string(body))
	}

	var tokenResp struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}

	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", err
	}

	token := tokenResp.Token
	if token == "" {
		token = tokenResp.AccessToken
	}

	return token, nil
}
