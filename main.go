package main

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-github/v81/github"
)

// リポジトリのコントリビューター情報
type Contributor struct {
	Avatar string `json:"avatar"`
	Name   string `json:"name"`
	URL    string `json:"url"`
}

// GitHub Trending APIからのリポジトリ情報
type TrendingRepo struct {
	Title         string        `json:"title"`
	URL           string        `json:"url"`
	Description   string        `json:"description"`
	Language      string        `json:"language,omitempty"`
	LanguageColor string        `json:"languageColor,omitempty"`
	Stars         string        `json:"stars"`
	Forks         string        `json:"forks"`
	AddStars      string        `json:"addStars"`
	Contributors  []Contributor `json:"contributors"`
}

// 要約を含むリポジトリ情報
type TrendingRepoWithSummary struct {
	Title         string        `json:"title"`
	URL           string        `json:"url"`
	Description   string        `json:"description"`
	Summary       string        `json:"summary"`
	Language      string        `json:"language,omitempty"`
	LanguageColor string        `json:"languageColor,omitempty"`
	Stars         string        `json:"stars"`
	Forks         string        `json:"forks"`
	AddStars      string        `json:"addStars"`
	Contributors  []Contributor `json:"contributors"`
}

// GitHub Trending APIのレスポンス
type TrendingAPIResponse struct {
	Items []TrendingRepo `json:"items"`
}

// 最終的なJSON出力構造
type Output struct {
	Items       []TrendingRepoWithSummary `json:"items"`
	GeneratedAt string                    `json:"generatedAt"`
}

// RSS構造体
type RSS struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel RSSChannel `xml:"channel"`
}

type RSSChannel struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	Language    string    `xml:"language"`
	PubDate     string    `xml:"pubDate"`
	Items       []RSSItem `xml:"item"`
}

type RSSItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	GUID        string `xml:"guid"`
	PubDate     string `xml:"pubDate"`
}

// Discord Webhook構造体
type DiscordWebhookPayload struct {
	Content string         `json:"content,omitempty"`
	Embeds  []DiscordEmbed `json:"embeds,omitempty"`
}

type DiscordEmbed struct {
	Title       string              `json:"title,omitempty"`
	Description string              `json:"description,omitempty"`
	URL         string              `json:"url,omitempty"`
	Color       int                 `json:"color,omitempty"`
	Fields      []DiscordEmbedField `json:"fields,omitempty"`
	Timestamp   string              `json:"timestamp,omitempty"`
}

type DiscordEmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

const (
	trendingAPIURL   = "https://raw.githubusercontent.com/isboyjc/github-trending-api/main/data/daily/all.json"
	outputPath       = "./public/data.json"
	feedPath         = "./public/feed.xml"
	siteURL          = "https://github-trending-ja.yashikota.com"
	defaultOllamaURL = "http://localhost:11434"
)

var httpClient = &http.Client{Timeout: 5 * time.Minute}

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatalf("error: %v", err)
	}
}

func run(ctx context.Context) error {
	// 1. Ollama設定取得
	ollamaURL := os.Getenv("OLLAMA_HOST")
	if ollamaURL == "" {
		ollamaURL = defaultOllamaURL
	}
	ollamaModel := os.Getenv("OLLAMA_MODEL")
	if ollamaModel == "" {
		return fmt.Errorf("OLLAMA_MODEL is not set")
	}
	log.Printf("Using Ollama at %s with model %s", ollamaURL, ollamaModel)

	// Discord Webhook設定取得（オプショナル）
	discordWebhookURL := os.Getenv("DISCORD_WEBHOOK_URL")
	if discordWebhookURL != "" {
		log.Println("Discord notification enabled")
	}

	// 2. GitHubクライアント初期化
	ghClient := github.NewClient(nil)

	// 3. Ollamaクライアント初期化
	ollamaClient := &OllamaClient{
		BaseURL: ollamaURL,
		Model:   ollamaModel,
		HTTP:    &http.Client{Timeout: 30 * time.Minute},
	}

	// 4. Trending取得
	log.Println("Fetching trending repositories...")
	repos, err := fetchTrendingRepos(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch trending repos: %w", err)
	}
	log.Printf("Found %d repositories", len(repos))

	// 5. 各リポジトリを処理
	results := make([]TrendingRepoWithSummary, 0, len(repos))
	for i, repo := range repos {
		log.Printf("[%d/%d] Processing %s...", i+1, len(repos), repo.Title)

		// owner/name 分解
		parts := strings.SplitN(repo.Title, "/", 2)
		if len(parts) != 2 {
			log.Printf("WARN: invalid title format: %s", repo.Title)
			continue
		}
		owner, name := parts[0], parts[1]

		// README取得
		readme, err := fetchReadme(ctx, ghClient, owner, name)
		if err != nil {
			log.Printf("WARN: failed to fetch README: %v", err)
			readme = repo.Description // fallback to description
		}

		// 要約生成
		summary, err := ollamaClient.Summarize(ctx, readme)
		if err != nil {
			log.Printf("WARN: failed to summarize: %v", err)
			summary = "要約失敗"
		}

		results = append(results, TrendingRepoWithSummary{
			Title:         repo.Title,
			URL:           repo.URL,
			Description:   repo.Description,
			Summary:       summary,
			Language:      repo.Language,
			LanguageColor: repo.LanguageColor,
			Stars:         repo.Stars,
			Forks:         repo.Forks,
			AddStars:      repo.AddStars,
			Contributors:  repo.Contributors,
		})
	}

	// 6. 出力
	generatedAt := time.Now().UTC()
	output := Output{
		Items:       results,
		GeneratedAt: generatedAt.Format(time.RFC3339),
	}

	if err := writeJSON(outputPath, output); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}
	log.Printf("Successfully wrote %d repositories to %s", len(results), outputPath)

	// 7. RSS生成
	if err := writeRSS(feedPath, results, generatedAt); err != nil {
		return fmt.Errorf("failed to write RSS: %w", err)
	}
	log.Printf("Successfully wrote RSS feed to %s", feedPath)

	// 8. Discord通知
	sendDiscordNotification(ctx, discordWebhookURL, results, generatedAt)

	return nil
}

func fetchTrendingRepos(ctx context.Context) ([]TrendingRepo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, trendingAPIURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var apiResp TrendingAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return apiResp.Items, nil
}

func fetchReadme(ctx context.Context, client *github.Client, owner, name string) (string, error) {
	readme, _, err := client.Repositories.GetReadme(ctx, owner, name, nil)
	if err != nil {
		return "", fmt.Errorf("get readme: %w", err)
	}

	content, err := readme.GetContent()
	if err != nil {
		return "", fmt.Errorf("get content: %w", err)
	}

	return content, nil
}

// OllamaClient はOllama APIクライアント
type OllamaClient struct {
	BaseURL string
	Model   string
	HTTP    *http.Client
}

// OllamaRequest はOllama APIへのリクエスト
type OllamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

// OllamaResponse はOllama APIからのレスポンス
type OllamaResponse struct {
	Response string `json:"response"`
}

// Summarize はREADMEを日本語で要約する
func (c *OllamaClient) Summarize(ctx context.Context, readme string) (string, error) {
	// READMEが空の場合
	if readme == "" {
		return "説明なし", nil
	}

	// READMEが長すぎる場合は切り詰め（トークン節約）
	const maxReadmeLen = 10000
	if len(readme) > maxReadmeLen {
		readme = readme[:maxReadmeLen]
	}

	prompt := fmt.Sprintf(
		"以下のREADMEの内容を日本語で短く要約せよ。100文字以内で\n\n%s",
		readme,
	)

	reqBody := OllamaRequest{
		Model:  c.Model,
		Prompt: prompt,
		Stream: false,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/generate", bytes.NewReader(jsonData))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var ollamaResp OllamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if ollamaResp.Response == "" {
		return "要約失敗", nil
	}

	return strings.TrimSpace(ollamaResp.Response), nil
}

func writeJSON(path string, data any) error {
	// ディレクトリ作成
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	// JSONエンコード
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)

	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("encode json: %w", err)
	}

	return nil
}

func writeRSS(path string, repos []TrendingRepoWithSummary, generatedAt time.Time) error {
	// ディレクトリ作成
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	// RSSアイテム作成
	items := make([]RSSItem, 0, len(repos))
	pubDate := generatedAt.Format(time.RFC1123Z)

	for _, repo := range repos {
		lang := "不明"
		if repo.Language != "" {
			lang = repo.Language
		}

		description := fmt.Sprintf(
			"%s<br><br>言語: %s<br>スター数: %s (+%s)<br>フォーク数: %s",
			html.EscapeString(repo.Summary),
			html.EscapeString(lang),
			html.EscapeString(repo.Stars),
			html.EscapeString(repo.AddStars),
			html.EscapeString(repo.Forks),
		)

		items = append(items, RSSItem{
			Title:       fmt.Sprintf("%s - %s", repo.Title, repo.Summary),
			Link:        repo.URL,
			Description: description,
			GUID:        fmt.Sprintf("%s-%s", repo.URL, generatedAt.Format("2006-01-02")),
			PubDate:     pubDate,
		})
	}

	rss := RSS{
		Version: "2.0",
		Channel: RSSChannel{
			Title:       "GitHub Trending 日本語まとめ",
			Link:        siteURL,
			Description: "1日のGitHub Trendingを日本語で紹介",
			Language:    "ja",
			PubDate:     pubDate,
			Items:       items,
		},
	}

	// ファイル作成
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer file.Close()

	// XML宣言を書き込み
	if _, err := file.WriteString(xml.Header); err != nil {
		return fmt.Errorf("write xml header: %w", err)
	}

	// XMLエンコード
	encoder := xml.NewEncoder(file)
	encoder.Indent("", "  ")

	if err := encoder.Encode(rss); err != nil {
		return fmt.Errorf("encode xml: %w", err)
	}

	return nil
}

// sendDiscordNotification はDiscord Webhookに通知を送信する
// エラーが発生しても処理は継続（ログ出力のみ）
func sendDiscordNotification(ctx context.Context, webhookURL string, repos []TrendingRepoWithSummary, generatedAt time.Time) {
	if webhookURL == "" {
		return
	}

	log.Println("Sending Discord notification...")

	// メッセージを分割して送信
	messages := buildDiscordMessages(repos, generatedAt)

	for i, msg := range messages {
		if err := postDiscordWebhook(ctx, webhookURL, msg); err != nil {
			log.Printf("WARN: failed to send Discord notification (%d/%d): %v", i+1, len(messages), err)
			continue
		}

		// Rate limit対策：複数メッセージ間に短い待機
		if i < len(messages)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}

	log.Println("Discord notification completed")
}

// postDiscordWebhook は単一のWebhookリクエストを送信する
func postDiscordWebhook(ctx context.Context, webhookURL string, payload DiscordWebhookPayload) error {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	return nil
}

// buildDiscordMessages はリポジトリ一覧をDiscordメッセージに変換する
func buildDiscordMessages(repos []TrendingRepoWithSummary, generatedAt time.Time) []DiscordWebhookPayload {
	const reposPerMessage = 1 // 1メッセージあたりのリポジトリ数

	var messages []DiscordWebhookPayload
	totalRepos := len(repos)

	for i := 0; i < totalRepos; i += reposPerMessage {
		end := i + reposPerMessage
		if end > totalRepos {
			end = totalRepos
		}

		batch := repos[i:end]
		embeds := make([]DiscordEmbed, 0, len(batch))

		// リポジトリ情報をEmbedに変換
		for _, repo := range batch {
			lang := repo.Language
			if lang == "" {
				lang = "不明"
			}

			embed := DiscordEmbed{
				Title:       repo.Title,
				URL:         repo.URL,
				Description: repo.Summary,
				Color:       languageToColor(repo.LanguageColor),
				Fields: []DiscordEmbedField{
					{Name: "言語", Value: lang, Inline: true},
					{Name: "スター", Value: fmt.Sprintf("%s (+%s)", repo.Stars, repo.AddStars), Inline: true},
				},
			}
			embeds = append(embeds, embed)
		}

		messages = append(messages, DiscordWebhookPayload{
			Embeds: embeds,
		})
	}

	return messages
}

// languageToColor はHTML色コードをDiscord色整数に変換
func languageToColor(htmlColor string) int {
	if htmlColor == "" {
		return 0x7289DA // Discord Blurple (default)
	}

	// "#RRGGBB" -> int
	color := strings.TrimPrefix(htmlColor, "#")
	var result int
	fmt.Sscanf(color, "%x", &result)
	return result
}
