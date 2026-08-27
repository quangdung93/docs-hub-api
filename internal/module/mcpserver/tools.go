package mcpserver

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/quangdung93/docs-hub-api/internal/common/contextx"
	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
	chatuc "github.com/quangdung93/docs-hub-api/internal/module/chat/usecase"
	projectdomain "github.com/quangdung93/docs-hub-api/internal/module/project/domain"
	retrievaldomain "github.com/quangdung93/docs-hub-api/internal/module/retrieval/domain"
	retrievaluc "github.com/quangdung93/docs-hub-api/internal/module/retrieval/usecase"
)

type listProjectsInput struct {
	Query string `json:"q,omitempty" jsonschema:"Optional case-insensitive project name or code filter"`
	Page  int    `json:"page,omitempty" jsonschema:"Page number, defaults to 1"`
}

type listVersionsInput struct {
	ProjectID string `json:"project_id" jsonschema:"Project UUID"`
	Page      int    `json:"page,omitempty" jsonschema:"Page number, defaults to 1"`
	Limit     int    `json:"limit,omitempty" jsonschema:"Items per page, maximum 100"`
}

type searchProjectInput struct {
	ProjectID string     `json:"project_id" jsonschema:"Project UUID"`
	Query     string     `json:"query" jsonschema:"Search question or keywords"`
	Scope     scopeInput `json:"scope,omitempty" jsonschema:"Version/change request scope; omitted means all"`
	Limit     int        `json:"limit,omitempty" jsonschema:"Maximum citations, from 1 to 50"`
}

type askProjectInput struct {
	ProjectID      string     `json:"project_id" jsonschema:"Project UUID"`
	Question       string     `json:"question" jsonschema:"Question grounded in project documents"`
	Scope          scopeInput `json:"scope,omitempty" jsonschema:"Version/change request scope; omitted means all"`
	ConversationID string     `json:"conversation_id,omitempty" jsonschema:"Existing conversation UUID; omitted creates one"`
}

type sourceInput struct {
	ProjectID  string `json:"project_id" jsonschema:"Project UUID"`
	DocumentID string `json:"document_id" jsonschema:"Document UUID"`
	RevisionID string `json:"revision_id" jsonschema:"Document revision UUID"`
	LineStart  int    `json:"line_start,omitempty" jsonschema:"First canonical line, one-based"`
	LineEnd    int    `json:"line_end,omitempty" jsonschema:"Last canonical line, inclusive"`
}

type compareVersionsInput struct {
	ProjectID        string   `json:"project_id" jsonschema:"Project UUID"`
	Question         string   `json:"question" jsonschema:"What should be compared across the selected timeline"`
	VersionIDs       []string `json:"version_ids,omitempty" jsonschema:"Project version UUIDs"`
	ChangeRequestIDs []string `json:"change_request_ids,omitempty" jsonschema:"Change request UUIDs"`
}

func (m *Module) registerTools() {
	mcp.AddTool(m.server, &mcp.Tool{
		Name: "list_projects", Description: "List projects accessible to the authenticated user.",
	}, m.listProjects)
	mcp.AddTool(m.server, &mcp.Tool{
		Name: "list_project_versions", Description: "List the version timeline of an accessible project.",
	}, m.listProjectVersions)
	mcp.AddTool(m.server, &mcp.Tool{
		Name:        "search_project",
		Description: "Search project documents in an explicit version/change-request scope and return citations.",
	}, m.searchProject)
	mcp.AddTool(m.server, &mcp.Tool{
		Name: "ask_project", Description: "Ask a grounded project question and return the answer with validated citations.",
	}, m.askProject)
	mcp.AddTool(m.server, &mcp.Tool{
		Name:        "get_document_source",
		Description: "Read a bounded canonical text range from an accessible ready document revision.",
	}, m.getDocumentSource)
	mcp.AddTool(m.server, &mcp.Tool{
		Name:        "compare_project_versions",
		Description: "Compare project knowledge across selected versions/change requests with citations.",
	}, m.compareProjectVersions)
}

func (m *Module) listProjects(
	ctx context.Context, _ *mcp.CallToolRequest, input listProjectsInput,
) (*mcp.CallToolResult, listProjectsOutput, error) {
	if err := m.allow(ctx, "list_projects"); err != nil {
		return nil, listProjectsOutput{}, err
	}
	actor, _ := contextx.ActorFrom(ctx)
	actorID, err := parseID(actor.UserID, "actor_id")
	if err != nil {
		return nil, listProjectsOutput{}, err
	}
	projects, err := m.allProjects(ctx, actorID)
	if err != nil {
		return nil, listProjectsOutput{}, safeError(ctx, "list_projects", err)
	}
	query := strings.ToLower(strings.TrimSpace(input.Query))
	filtered := make([]projectdomain.Project, 0, len(projects))
	for _, project := range projects {
		if query == "" || strings.Contains(strings.ToLower(project.Name), query) ||
			strings.Contains(strings.ToLower(project.Code), query) {
			filtered = append(filtered, project)
		}
	}
	page := input.Page
	if page < 1 {
		page = 1
	}
	const limit = 20
	start := (page - 1) * limit
	end := min(start+limit, len(filtered))
	items := []projectSummary{}
	if start < len(filtered) {
		items = make([]projectSummary, 0, end-start)
		for _, project := range filtered[start:end] {
			items = append(items, toProjectSummary(project))
		}
	}
	return nil, listProjectsOutput{
		Projects: items, Page: pagination.NewMeta(page, limit, int64(len(filtered))),
	}, nil
}

func (m *Module) allProjects(ctx context.Context, actorID uuid.UUID) ([]projectdomain.Project, error) {
	items := make([]projectdomain.Project, 0)
	for page := 1; page <= 100; page++ {
		batch, meta, err := m.projects.List(ctx, actorID, pagination.Query{Page: page, Limit: 100})
		if err != nil {
			return nil, err
		}
		items = append(items, batch...)
		if !meta.HasNext {
			break
		}
	}
	return items, nil
}

func (m *Module) listProjectVersions(
	ctx context.Context, _ *mcp.CallToolRequest, input listVersionsInput,
) (*mcp.CallToolResult, listVersionsOutput, error) {
	if err := m.allow(ctx, "list_project_versions"); err != nil {
		return nil, listVersionsOutput{}, err
	}
	projectID, err := parseID(input.ProjectID, "project_id")
	if err != nil {
		return nil, listVersionsOutput{}, err
	}
	versions, meta, err := m.projects.ListVersions(ctx, projectID, pagination.Query{Page: input.Page, Limit: input.Limit})
	if err != nil {
		return nil, listVersionsOutput{}, safeError(ctx, "list_project_versions", err)
	}
	out := make([]versionSummary, 0, len(versions))
	for _, version := range versions {
		out = append(out, toVersionSummary(version))
	}
	m.audit(ctx, "list_project_versions", projectID)
	return nil, listVersionsOutput{Versions: out, Page: meta}, nil
}

func (m *Module) searchProject(
	ctx context.Context, _ *mcp.CallToolRequest, input searchProjectInput,
) (*mcp.CallToolResult, searchOutput, error) {
	if err := m.allow(ctx, "search_project"); err != nil {
		return nil, searchOutput{}, err
	}
	projectID, err := parseID(input.ProjectID, "project_id")
	if err != nil {
		return nil, searchOutput{}, err
	}
	scope, err := parseScope(input.Scope)
	if err != nil {
		return nil, searchOutput{}, err
	}
	result, err := m.retrieval.Retrieve(ctx, retrievaluc.Input{
		ProjectID: projectID, Query: input.Query, Scope: scope, PageSize: input.Limit,
	})
	if err != nil {
		return nil, searchOutput{}, safeError(ctx, "search_project", err)
	}
	m.audit(ctx, "search_project", projectID)
	return nil, searchOutput{
		Query: result.Query, ResolvedScope: resolvedScopes(result.ResolvedScope),
		Citations: retrievalCitations(result.Citations), Total: result.Total,
	}, nil
}

func (m *Module) askProject(
	ctx context.Context, _ *mcp.CallToolRequest, input askProjectInput,
) (*mcp.CallToolResult, answerOutput, error) {
	if err := m.allow(ctx, "ask_project"); err != nil {
		return nil, answerOutput{}, err
	}
	projectID, err := parseID(input.ProjectID, "project_id")
	if err != nil {
		return nil, answerOutput{}, err
	}
	scope, err := parseScope(input.Scope)
	if err != nil {
		return nil, answerOutput{}, err
	}
	output, err := m.runAsk(ctx, projectID, input.ConversationID, input.Question, scope)
	if err != nil {
		return nil, answerOutput{}, err
	}
	m.audit(ctx, "ask_project", projectID)
	return nil, *output, nil
}

func (m *Module) getDocumentSource(
	ctx context.Context, _ *mcp.CallToolRequest, input sourceInput,
) (*mcp.CallToolResult, sourceOutput, error) {
	if err := m.allow(ctx, "get_document_source"); err != nil {
		return nil, sourceOutput{}, err
	}
	projectID, err := parseID(input.ProjectID, "project_id")
	if err != nil {
		return nil, sourceOutput{}, err
	}
	documentID, err := parseID(input.DocumentID, "document_id")
	if err != nil {
		return nil, sourceOutput{}, err
	}
	revisionID, err := parseID(input.RevisionID, "revision_id")
	if err != nil {
		return nil, sourceOutput{}, err
	}
	output, err := m.readSource(ctx, projectID, documentID, revisionID, input.LineStart, input.LineEnd)
	if err != nil {
		return nil, sourceOutput{}, err
	}
	m.audit(ctx, "get_document_source", projectID)
	return nil, *output, nil
}

func (m *Module) compareProjectVersions(
	ctx context.Context, _ *mcp.CallToolRequest, input compareVersionsInput,
) (*mcp.CallToolResult, answerOutput, error) {
	if err := m.allow(ctx, "compare_project_versions"); err != nil {
		return nil, answerOutput{}, err
	}
	projectID, err := parseID(input.ProjectID, "project_id")
	if err != nil {
		return nil, answerOutput{}, err
	}
	scopeInput := scopeInput{VersionIDs: input.VersionIDs, ChangeRequestIDs: input.ChangeRequestIDs}
	if len(input.VersionIDs) > 0 && len(input.ChangeRequestIDs) > 0 {
		return nil, answerOutput{}, errors.New("REQ_400: Một lần so sánh chỉ chọn versions hoặc change requests")
	}
	scope, err := parseScope(scopeInput)
	if err != nil {
		return nil, answerOutput{}, err
	}
	output, err := m.runAsk(ctx, projectID, "", input.Question, scope)
	if err != nil {
		return nil, answerOutput{}, err
	}
	m.audit(ctx, "compare_project_versions", projectID)
	return nil, *output, nil
}

func (m *Module) runAsk(
	ctx context.Context, projectID uuid.UUID, rawConversationID, question string, scope retrievaldomain.Scope,
) (*answerOutput, error) {
	var conversationID uuid.UUID
	var err error
	if strings.TrimSpace(rawConversationID) == "" {
		conversation, createErr := m.chat.Create(ctx, chatuc.CreateInput{
			ProjectID: projectID, Title: conversationTitle(question), Scope: &scope,
		})
		if createErr != nil {
			return nil, safeError(ctx, "ask_project", createErr)
		}
		conversationID = conversation.ID
	} else {
		conversationID, err = parseID(rawConversationID, "conversation_id")
		if err != nil {
			return nil, err
		}
	}
	answer, err := m.chat.Ask(ctx, chatuc.AskInput{
		ProjectID: projectID, ConversationID: conversationID, Question: question, Scope: &scope,
	})
	if err != nil {
		return nil, safeError(ctx, "ask_project", err)
	}
	return &answerOutput{
		ConversationID: conversationID.String(), Answer: answer.Answer, Intent: answer.Intent,
		ResolvedScope: resolvedScopes(answer.ResolvedScope), Citations: chatCitations(answer.Citations),
		Grounded: answer.Grounded,
	}, nil
}

func toProjectSummary(project projectdomain.Project) projectSummary {
	return projectSummary{
		ID: project.ID.String(), Code: project.Code, Name: project.Name, Description: project.Description,
		Status: project.Status, Version: project.Version, UpdatedAt: project.UpdatedAt,
	}
}

func toVersionSummary(version projectdomain.ProjectVersion) versionSummary {
	return versionSummary{
		ID: version.ID.String(), ProjectID: version.ProjectID.String(), Label: version.Label,
		SequenceNo: version.SequenceNo, Status: version.Status, ReleasedAt: version.ReleasedAt,
	}
}

func conversationTitle(question string) string {
	runes := []rune(strings.TrimSpace(question))
	if len(runes) > 100 {
		runes = runes[:100]
	}
	return string(runes)
}
