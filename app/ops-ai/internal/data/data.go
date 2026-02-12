package data

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/littleSand/adama/app/ops-ai/internal/conf"
	"github.com/littleSand/adama/pkg/envutil"

	_ "github.com/go-sql-driver/mysql"
)

type CorpusChunk struct {
	Tool    string `json:"tool"`
	Source  string `json:"source"`
	Snippet string `json:"snippet"`
	Score   int    `json:"score"`
}

type ToolQuery struct {
	Question string
	Params   map[string]string
	Limit    int
}

type WorkflowRecord struct {
	OrderID     int64  `json:"order_id"`
	UserID      int64  `json:"user_id"`
	GoodsID     int64  `json:"goods_id"`
	Amount      int64  `json:"amount"`
	Status      string `json:"status"`
	StockStatus string `json:"stock_status"`
	CacheStatus string `json:"cache_status"`
	SyncStatus  string `json:"sync_status"`
	LastError   string `json:"last_error"`
}

type StockReservationRecord struct {
	OrderID int64  `json:"order_id"`
	GoodsID int64  `json:"goods_id"`
	Amount  int64  `json:"amount"`
	Status  string `json:"status"`
}

type OpenAIClient struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

type Data struct {
	log           *log.Helper
	db            *sql.DB
	rootDir       string
	faqChunks     []CorpusChunk
	docChunks     []CorpusChunk
	benchmarkDocs []CorpusChunk
	openai        *OpenAIClient
}

func NewData(cfg *conf.Data, logger log.Logger) (*Data, func(), error) {
	helper := log.NewHelper(log.With(logger, "module", "ops-ai/data"))
	rootDir, err := detectRepoRoot()
	if err != nil {
		return nil, nil, err
	}

	var db *sql.DB
	databaseSource := envutil.Get("MYSQL_DSN", cfg.Database.Source)
	if cfg.Database.Driver != "" && databaseSource != "" {
		db, err = sql.Open(cfg.Database.Driver, databaseSource)
		if err != nil {
			return nil, nil, err
		}
	}

	faqChunks, err := loadMarkdownChunks(rootDir, []documentSource{
		{Tool: "search_faq", Path: "FQA.md"},
		{Tool: "search_docs", Path: "scheduled.md"},
	})
	if err != nil {
		return nil, nil, err
	}
	docChunks, err := loadDirChunks(rootDir, "docs", "search_docs")
	if err != nil {
		return nil, nil, err
	}
	benchmarkChunks, err := loadDirChunks(rootDir, "benchmarks", "query_benchmark_history")
	if err != nil {
		return nil, nil, err
	}

	data := &Data{
		log:           helper,
		db:            db,
		rootDir:       rootDir,
		faqChunks:     faqChunks,
		docChunks:     docChunks,
		benchmarkDocs: benchmarkChunks,
		openai:        newOpenAIClient(cfg.OpenAI),
	}

	return data, func() {
		if data.db != nil {
			_ = data.db.Close()
		}
	}, nil
}

type documentSource struct {
	Tool string
	Path string
}

func loadMarkdownChunks(rootDir string, sources []documentSource) ([]CorpusChunk, error) {
	chunks := make([]CorpusChunk, 0)
	for _, source := range sources {
		fullPath := filepath.Join(rootDir, source.Path)
		content, err := ioutil.ReadFile(fullPath)
		if err != nil {
			return nil, err
		}
		chunks = append(chunks, splitIntoChunks(source.Tool, source.Path, string(content))...)
	}
	return chunks, nil
}

func loadDirChunks(rootDir, dir, tool string) ([]CorpusChunk, error) {
	chunks := make([]CorpusChunk, 0)
	fullDir := filepath.Join(rootDir, dir)
	err := filepath.Walk(fullDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".md" && ext != ".txt" && ext != ".log" && ext != ".json" && ext != ".lua" {
			return nil
		}
		content, readErr := ioutil.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		rel, relErr := filepath.Rel(rootDir, path)
		if relErr != nil {
			rel = path
		}
		chunks = append(chunks, splitIntoChunks(tool, filepath.ToSlash(rel), string(content))...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return chunks, nil
}

func splitIntoChunks(tool, source, content string) []CorpusChunk {
	rawParts := strings.Split(content, "\n\n")
	chunks := make([]CorpusChunk, 0, len(rawParts))
	for _, part := range rawParts {
		snippet := strings.TrimSpace(part)
		if snippet == "" {
			continue
		}
		if len(snippet) > 1200 {
			snippet = snippet[:1200]
		}
		chunks = append(chunks, CorpusChunk{
			Tool:    tool,
			Source:  filepath.ToSlash(source),
			Snippet: snippet,
		})
	}
	return chunks
}

func detectRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("repo root not found")
		}
		dir = parent
	}
}

func (d *Data) SearchFAQ(_ context.Context, query ToolQuery) []CorpusChunk {
	return rankChunks(d.faqChunks, query.Question, query.Limit, "search_faq")
}

func (d *Data) SearchDocs(_ context.Context, query ToolQuery) []CorpusChunk {
	return rankChunks(d.docChunks, query.Question, query.Limit, "search_docs")
}

func (d *Data) QueryBenchmarkHistory(_ context.Context, query ToolQuery) []CorpusChunk {
	return rankChunks(d.benchmarkDocs, query.Question, query.Limit, "query_benchmark_history")
}

func rankChunks(chunks []CorpusChunk, question string, limit int, tool string) []CorpusChunk {
	if limit <= 0 {
		limit = 4
	}
	tokens := tokenize(question)
	scored := make([]CorpusChunk, 0, len(chunks))
	for _, chunk := range chunks {
		score := chunkScore(chunk.Snippet, question, tokens)
		if score == 0 {
			continue
		}
		copyChunk := chunk
		copyChunk.Tool = tool
		copyChunk.Score = score
		scored = append(scored, copyChunk)
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].Score == scored[j].Score {
			return scored[i].Source < scored[j].Source
		}
		return scored[i].Score > scored[j].Score
	})
	if len(scored) > limit {
		scored = scored[:limit]
	}
	return scored
}

func tokenize(question string) []string {
	replacer := strings.NewReplacer(",", " ", ".", " ", "?", " ", "，", " ", "。", " ", "：", " ", ":", " ", "\n", " ")
	fields := strings.Fields(strings.ToLower(replacer.Replace(question)))
	if len(fields) == 0 && question != "" {
		return []string{strings.ToLower(question)}
	}
	return fields
}

func chunkScore(snippet, question string, tokens []string) int {
	text := strings.ToLower(snippet)
	score := 0
	if question != "" && strings.Contains(text, strings.ToLower(question)) {
		score += 5
	}
	for _, token := range tokens {
		if token == "" {
			continue
		}
		if strings.Contains(text, token) {
			score += 2
		}
	}
	return score
}

func (d *Data) QueryOrderWorkflow(ctx context.Context, query ToolQuery) ([]WorkflowRecord, error) {
	if d.db == nil {
		return nil, nil
	}
	limit := query.Limit
	if limit <= 0 || limit > 10 {
		limit = 5
	}
	rows, err := d.db.QueryContext(ctx, `
		SELECT order_id, user_id, goods_id, amount, status, stock_status, cache_status, sync_status, last_error
		FROM adama_order_workflows
		ORDER BY updated_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]WorkflowRecord, 0, limit)
	for rows.Next() {
		var item WorkflowRecord
		if err := rows.Scan(&item.OrderID, &item.UserID, &item.GoodsID, &item.Amount, &item.Status, &item.StockStatus, &item.CacheStatus, &item.SyncStatus, &item.LastError); err != nil {
			return nil, err
		}
		records = append(records, item)
	}
	return records, rows.Err()
}

func (d *Data) QueryStockReservation(ctx context.Context, query ToolQuery) ([]StockReservationRecord, error) {
	if d.db == nil {
		return nil, nil
	}
	limit := query.Limit
	if limit <= 0 || limit > 10 {
		limit = 5
	}
	rows, err := d.db.QueryContext(ctx, `
		SELECT order_id, goods_id, amount, status
		FROM adama_stock_reservations
		ORDER BY updated_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]StockReservationRecord, 0, limit)
	for rows.Next() {
		var item StockReservationRecord
		if err := rows.Scan(&item.OrderID, &item.GoodsID, &item.Amount, &item.Status); err != nil {
			return nil, err
		}
		records = append(records, item)
	}
	return records, rows.Err()
}

func (d *Data) QueryRecentErrors(ctx context.Context, query ToolQuery) ([]WorkflowRecord, error) {
	if d.db == nil {
		return nil, nil
	}
	limit := query.Limit
	if limit <= 0 || limit > 10 {
		limit = 5
	}
	rows, err := d.db.QueryContext(ctx, `
		SELECT order_id, user_id, goods_id, amount, status, stock_status, cache_status, sync_status, last_error
		FROM adama_order_workflows
		WHERE last_error <> ''
		ORDER BY updated_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]WorkflowRecord, 0, limit)
	for rows.Next() {
		var item WorkflowRecord
		if err := rows.Scan(&item.OrderID, &item.UserID, &item.GoodsID, &item.Amount, &item.Status, &item.StockStatus, &item.CacheStatus, &item.SyncStatus, &item.LastError); err != nil {
			return nil, err
		}
		records = append(records, item)
	}
	return records, rows.Err()
}

func newOpenAIClient(cfg conf.OpenAI) *OpenAIClient {
	if cfg.APIKey == "" || cfg.BaseURL == "" {
		return nil
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	model := cfg.Model
	if model == "" {
		model = "gpt-5-mini"
	}
	return &OpenAIClient{
		baseURL: cfg.BaseURL,
		apiKey:  cfg.APIKey,
		model:   model,
		client:  &http.Client{Timeout: timeout},
	}
}

type responseRequest struct {
	Model string        `json:"model"`
	Input []inputObject `json:"input"`
}

type inputObject struct {
	Role    string         `json:"role"`
	Content []contentBlock `json:"content"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type responseEnvelope struct {
	OutputText string `json:"output_text"`
	Output     []struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
}

func (d *Data) GenerateConclusion(ctx context.Context, systemPrompt, userPrompt, model string) (string, error) {
	if d.openai == nil {
		return "", nil
	}
	return d.openai.GenerateConclusion(ctx, systemPrompt, userPrompt, model)
}

func (c *OpenAIClient) GenerateConclusion(ctx context.Context, systemPrompt, userPrompt, model string) (string, error) {
	if model == "" {
		model = c.model
	}
	reqBody := responseRequest{
		Model: model,
		Input: []inputObject{
			{
				Role: "system",
				Content: []contentBlock{{
					Type: "input_text",
					Text: systemPrompt,
				}},
			},
			{
				Role: "user",
				Content: []contentBlock{{
					Type: "input_text",
					Text: userPrompt,
				}},
			},
		},
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+c.apiKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := c.client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode >= 300 {
		body, _ := ioutil.ReadAll(response.Body)
		return "", fmt.Errorf("openai responses api status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	var envelope responseEnvelope
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return "", err
	}
	if strings.TrimSpace(envelope.OutputText) != "" {
		return strings.TrimSpace(envelope.OutputText), nil
	}
	for _, output := range envelope.Output {
		for _, content := range output.Content {
			if strings.TrimSpace(content.Text) != "" {
				return strings.TrimSpace(content.Text), nil
			}
		}
	}
	return "", nil
}
