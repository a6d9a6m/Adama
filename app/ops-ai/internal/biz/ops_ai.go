package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/littleSand/adama/app/ops-ai/internal/data"
)

type Repository interface {
	SearchFAQ(ctx context.Context, query data.ToolQuery) []data.CorpusChunk
	SearchDocs(ctx context.Context, query data.ToolQuery) []data.CorpusChunk
	QueryBenchmarkHistory(ctx context.Context, query data.ToolQuery) []data.CorpusChunk
	QueryOrderWorkflow(ctx context.Context, query data.ToolQuery) ([]data.WorkflowRecord, error)
	QueryStockReservation(ctx context.Context, query data.ToolQuery) ([]data.StockReservationRecord, error)
	QueryRecentErrors(ctx context.Context, query data.ToolQuery) ([]data.WorkflowRecord, error)
	GenerateConclusion(ctx context.Context, systemPrompt, userPrompt, model string) (string, error)
}

type AskRequest struct {
	Question          string            `json:"question"`
	SceneTags         []string          `json:"scene_tags"`
	AllowDynamicQuery bool              `json:"allow_dynamic_query"`
	Model             string            `json:"model"`
	Params            map[string]string `json:"params"`
}

type Evidence struct {
	Tool    string      `json:"tool"`
	Source  string      `json:"source"`
	Snippet string      `json:"snippet,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

type AskResponse struct {
	Conclusion    string     `json:"conclusion"`
	Basis         []Evidence `json:"basis"`
	Suggestions   []string   `json:"suggestions"`
	ToolsUsed     []string   `json:"tools_used"`
	AnswerType    string     `json:"answer_type"`
	ModelDecision string     `json:"model_decision"`
}

type OpsAIUsecase struct {
	repo Repository
	log  *log.Helper
}

func NewOpsAIUsecase(repo Repository, logger log.Logger) *OpsAIUsecase {
	return &OpsAIUsecase{
		repo: repo,
		log:  log.NewHelper(logger),
	}
}

func (uc *OpsAIUsecase) Ask(ctx context.Context, req AskRequest) (*AskResponse, error) {
	query := data.ToolQuery{
		Question: req.Question,
		Params:   req.Params,
		Limit:    4,
	}

	basis := make([]Evidence, 0, 8)
	toolsUsed := make([]string, 0, 6)
	seenTools := map[string]struct{}{}

	appendTool := func(name string) {
		if _, ok := seenTools[name]; ok {
			return
		}
		seenTools[name] = struct{}{}
		toolsUsed = append(toolsUsed, name)
	}

	for _, item := range uc.repo.SearchFAQ(ctx, query) {
		appendTool("search_faq")
		basis = append(basis, Evidence{Tool: item.Tool, Source: item.Source, Snippet: item.Snippet})
	}
	for _, item := range uc.repo.SearchDocs(ctx, query) {
		appendTool("search_docs")
		basis = append(basis, Evidence{Tool: item.Tool, Source: item.Source, Snippet: item.Snippet})
	}

	if matchesAny(req.Question, req.SceneTags, "benchmark", "压测", "wrk", "性能") {
		for _, item := range uc.repo.QueryBenchmarkHistory(ctx, query) {
			appendTool("query_benchmark_history")
			basis = append(basis, Evidence{Tool: item.Tool, Source: item.Source, Snippet: item.Snippet})
		}
	}

	if req.AllowDynamicQuery {
		if records, err := uc.repo.QueryOrderWorkflow(ctx, query); err == nil && len(records) > 0 {
			appendTool("query_order_workflow")
			basis = append(basis, Evidence{Tool: "query_order_workflow", Source: "mysql:adama_order_workflows", Data: records})
		}
		if records, err := uc.repo.QueryStockReservation(ctx, query); err == nil && len(records) > 0 {
			appendTool("query_stock_reservation")
			basis = append(basis, Evidence{Tool: "query_stock_reservation", Source: "mysql:adama_stock_reservations", Data: records})
		}
		if records, err := uc.repo.QueryRecentErrors(ctx, query); err == nil && len(records) > 0 {
			appendTool("query_recent_errors")
			basis = append(basis, Evidence{Tool: "query_recent_errors", Source: "mysql:adama_order_workflows", Data: records})
		}
	}

	conclusion, answerType, modelDecision, err := uc.buildConclusion(ctx, req, basis)
	if err != nil {
		return nil, err
	}

	return &AskResponse{
		Conclusion:    conclusion,
		Basis:         basis,
		Suggestions:   buildSuggestions(req, basis),
		ToolsUsed:     toolsUsed,
		AnswerType:    answerType,
		ModelDecision: modelDecision,
	}, nil
}

func (uc *OpsAIUsecase) buildConclusion(ctx context.Context, req AskRequest, basis []Evidence) (string, string, string, error) {
	systemPrompt := "你是 adama 项目的运维问答助手。只能基于给定依据回答；无法确认的内容要明确说不知道，并区分“文档结论”和“模型推断”。禁止建议任何直接写库或执行 shell 的动作。"
	userPrompt := buildPrompt(req, basis)

	output, err := uc.repo.GenerateConclusion(ctx, systemPrompt, userPrompt, req.Model)
	if err != nil {
		return "", "", "", err
	}
	if strings.TrimSpace(output) != "" {
		return output, "model_inference", "openai_responses_api", nil
	}

	if len(basis) == 0 {
		return "未找到足够依据。当前第一阶段只接入了 FAQ、docs、benchmarks 和只读工作流查询。", "document_conclusion", "local_retrieval_only", nil
	}

	first := basis[0]
	if first.Snippet != "" {
		return fmt.Sprintf("根据 %s 的检索结果，当前最相关依据是：%s", first.Source, compress(first.Snippet, 180)), "document_conclusion", "local_retrieval_only", nil
	}
	payload, _ := json.Marshal(first.Data)
	return fmt.Sprintf("根据 %s 的只读查询结果，当前最相关数据为：%s", first.Source, compress(string(payload), 180)), "document_conclusion", "local_retrieval_only", nil
}

func buildPrompt(req AskRequest, basis []Evidence) string {
	var builder strings.Builder
	builder.WriteString("问题：")
	builder.WriteString(req.Question)
	builder.WriteString("\n")
	if len(req.SceneTags) > 0 {
		builder.WriteString("场景标签：")
		builder.WriteString(strings.Join(req.SceneTags, ", "))
		builder.WriteString("\n")
	}
	builder.WriteString("依据：\n")
	for _, item := range basis {
		builder.WriteString("- 工具: ")
		builder.WriteString(item.Tool)
		builder.WriteString(" 来源: ")
		builder.WriteString(item.Source)
		builder.WriteString("\n")
		if item.Snippet != "" {
			builder.WriteString(item.Snippet)
			builder.WriteString("\n")
			continue
		}
		raw, _ := json.Marshal(item.Data)
		builder.Write(raw)
		builder.WriteString("\n")
	}
	builder.WriteString("输出 JSON 以外的自然语言即可，先给结论，再给依据，再给建议动作。")
	return builder.String()
}

func buildSuggestions(req AskRequest, basis []Evidence) []string {
	suggestions := []string{
		"优先根据 basis 中的文档或只读查询结果确认结论，不要直接执行写操作。",
	}
	if req.AllowDynamicQuery {
		suggestions = append(suggestions, "如果需要更精确定位，可在 params 中补充 order_id、goods_id 或 limit。")
	}
	if len(basis) == 0 {
		suggestions = append(suggestions, "补充更具体的问题关键词，或打开 allow_dynamic_query 以读取工作流状态。")
	}
	return suggestions
}

func matchesAny(question string, tags []string, words ...string) bool {
	text := strings.ToLower(question + " " + strings.Join(tags, " "))
	for _, word := range words {
		if strings.Contains(text, strings.ToLower(word)) {
			return true
		}
	}
	return false
}

func compress(text string, limit int) string {
	text = strings.TrimSpace(text)
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "..."
}
