package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
)

func (m *Module) registerResources() {
	m.server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "project://{project_id}", Name: "project_metadata", MIMEType: "application/json",
		Description: "Metadata of a project accessible to the authenticated user.",
	}, m.readResource)
	m.server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "project://{project_id}/versions/{version_id}", Name: "project_version",
		MIMEType: "application/json", Description: "Metadata of one project version.",
	}, m.readResource)
	m.server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "project://{project_id}/documents/{document_id}/revisions/{revision_id}",
		Name:        "document_revision_source", MIMEType: "text/plain",
		Description: "Bounded canonical text of a ready document revision.",
	}, m.readResource)
}

func (m *Module) readResource(ctx context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	if err := m.allow(ctx, "read_resource"); err != nil {
		return nil, err
	}
	uri := request.Params.URI
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme != "project" || parsed.Host == "" || parsed.RawQuery != "" {
		return nil, resourceNotFound(uri)
	}
	projectID, err := parseID(parsed.Host, "project_id")
	if err != nil {
		return nil, resourceNotFound(uri)
	}
	segments := splitPath(parsed.Path)
	switch {
	case len(segments) == 0:
		return m.readProjectResource(ctx, uri, projectID)
	case len(segments) == 2 && segments[0] == "versions":
		return m.readVersionResource(ctx, uri, projectID, segments[1])
	case len(segments) == 4 && segments[0] == "documents" && segments[2] == "revisions":
		return m.readDocumentResource(ctx, uri, projectID, segments[1], segments[3])
	default:
		return nil, resourceNotFound(uri)
	}
}

func (m *Module) readProjectResource(
	ctx context.Context, uri string, projectID uuid.UUID,
) (*mcp.ReadResourceResult, error) {
	project, err := m.projects.GetByID(ctx, projectID)
	if err != nil {
		return nil, safeError(ctx, "read_project_resource", err)
	}
	text, err := marshalResource(toProjectSummary(*project))
	if err != nil {
		return nil, safeError(ctx, "read_project_resource", err)
	}
	m.audit(ctx, "read_project_resource", projectID)
	return textResource(uri, "application/json", text), nil
}

func (m *Module) readVersionResource(
	ctx context.Context, uri string, projectID uuid.UUID, rawVersionID string,
) (*mcp.ReadResourceResult, error) {
	versionID, err := parseID(rawVersionID, "version_id")
	if err != nil {
		return nil, resourceNotFound(uri)
	}
	version, err := m.findVersion(ctx, projectID, versionID)
	if err != nil {
		return nil, safeError(ctx, "read_version_resource", err)
	}
	if version == nil {
		return nil, resourceNotFound(uri)
	}
	text, err := marshalResource(*version)
	if err != nil {
		return nil, safeError(ctx, "read_version_resource", err)
	}
	m.audit(ctx, "read_version_resource", projectID)
	return textResource(uri, "application/json", text), nil
}

func (m *Module) readDocumentResource(
	ctx context.Context, uri string, projectID uuid.UUID, rawDocumentID, rawRevisionID string,
) (*mcp.ReadResourceResult, error) {
	documentID, err := parseID(rawDocumentID, "document_id")
	if err != nil {
		return nil, resourceNotFound(uri)
	}
	revisionID, err := parseID(rawRevisionID, "revision_id")
	if err != nil {
		return nil, resourceNotFound(uri)
	}
	source, err := m.readSource(ctx, projectID, documentID, revisionID, 1, m.maxSourceLines)
	if err != nil {
		return nil, err
	}
	m.audit(ctx, "read_document_resource", projectID)
	return textResource(uri, "text/plain", source.Text), nil
}

func (m *Module) findVersion(ctx context.Context, projectID, versionID uuid.UUID) (*versionSummary, error) {
	for page := 1; page <= 100; page++ {
		versions, meta, err := m.projects.ListVersions(ctx, projectID, pagination.Query{Page: page, Limit: 100})
		if err != nil {
			return nil, err
		}
		for _, version := range versions {
			if version.ID == versionID {
				result := toVersionSummary(version)
				return &result, nil
			}
		}
		if !meta.HasNext {
			break
		}
	}
	return nil, nil
}

func splitPath(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func marshalResource(value any) (string, error) {
	data, err := json.Marshal(value)
	return string(data), err
}

func textResource(uri, mimeType, text string) *mcp.ReadResourceResult {
	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: uri, MIMEType: mimeType, Text: text}}}
}

func resourceNotFound(uri string) error {
	return fmt.Errorf("resource không tồn tại: %w", mcp.ResourceNotFoundError(uri))
}
