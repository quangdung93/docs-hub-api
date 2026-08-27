// Package mcpserver cung cấp delivery adapter MCP read-only trên Streamable HTTP.
package mcpserver

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
	"github.com/quangdung93/docs-hub-api/internal/common/port"
	chatdomain "github.com/quangdung93/docs-hub-api/internal/module/chat/domain"
	chatuc "github.com/quangdung93/docs-hub-api/internal/module/chat/usecase"
	documentdomain "github.com/quangdung93/docs-hub-api/internal/module/document/domain"
	projectdomain "github.com/quangdung93/docs-hub-api/internal/module/project/domain"
	retrievaluc "github.com/quangdung93/docs-hub-api/internal/module/retrieval/usecase"
)

const serverVersion = "v1"

// ProjectService là phần application contract MCP cần từ module project.
type ProjectService interface {
	List(context.Context, uuid.UUID, pagination.Query) ([]projectdomain.Project, pagination.Meta, error)
	GetByID(context.Context, uuid.UUID) (*projectdomain.Project, error)
	ListVersions(context.Context, uuid.UUID, pagination.Query) ([]projectdomain.ProjectVersion, pagination.Meta, error)
}

// RetrievalService là application contract tìm kiếm có ACL/scope.
type RetrievalService interface {
	Retrieve(context.Context, retrievaluc.Input) (*retrievaluc.Result, error)
}

// ChatService là application contract tạo hội thoại và hỏi đáp có citation.
type ChatService interface {
	Create(context.Context, chatuc.CreateInput) (*chatdomain.Conversation, error)
	Ask(context.Context, chatuc.AskInput) (*chatuc.Answer, error)
}

// DocumentService là application contract đọc canonical source có ACL.
type DocumentService interface {
	CanonicalSource(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*documentdomain.Revision, io.ReadCloser, error)
}

// Deps gom usecase và giới hạn bảo mật của MCP adapter.
type Deps struct {
	Projects          ProjectService
	Retrieval         RetrievalService
	Chat              ChatService
	Documents         DocumentService
	Cache             port.Cache
	Auditor           port.Auditor
	RequestsPerWindow int
	Window            time.Duration
	MaxSourceLines    int
	MaxExcerptChars   int
}

// Module giữ MCP server và HTTP transport handler.
type Module struct {
	server            *mcp.Server
	handler           http.Handler
	projects          ProjectService
	retrieval         RetrievalService
	chat              ChatService
	documents         DocumentService
	cache             port.Cache
	auditor           port.Auditor
	requestsPerWindow int
	window            time.Duration
	maxSourceLines    int
	maxExcerptChars   int
}

// New dựng MCP server stateless; auth được bootstrap bọc bên ngoài handler.
func New(deps Deps) *Module {
	module := &Module{
		projects: deps.Projects, retrieval: deps.Retrieval, chat: deps.Chat, documents: deps.Documents,
		cache: deps.Cache, auditor: deps.Auditor, requestsPerWindow: deps.RequestsPerWindow,
		window: deps.Window, maxSourceLines: deps.MaxSourceLines, maxExcerptChars: deps.MaxExcerptChars,
	}
	module.server = mcp.NewServer(&mcp.Implementation{Name: "docs-hub-api", Version: serverVersion}, nil)
	module.registerTools()
	module.registerResources()
	streamable := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return module.server
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	module.handler = new(http.CrossOriginProtection).Handler(streamable)
	return module
}

// Handler trả transport handler để bootstrap mount tại POST /mcp.
func (m *Module) Handler() http.Handler { return m.handler }
