// Package ragflow hiện thực RAGFlow HTTP API. Package này chỉ chứa transport
// DTO; domain/usecase sử dụng interface và type trong internal/common/port.
package ragflow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/quangdung93/docs-hub-api/internal/common/port"
)

const maxResponseBytes = 8 << 20

// codeNotOwned là mã RAGFlow trả khi tên không thuộc sở hữu của user hiện tại.
// RAGFlow dùng CHUNG mã này cho cả "không tồn tại" lẫn "của người khác" — đã
// kiểm chứng bằng một tên tự bịa ra, vẫn trả đúng mã này.
const codeNotOwned = 102

type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
	upload  *http.Client
}

func New(baseURL, apiKey string, timeout, uploadTimeout time.Duration) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  strings.TrimSpace(apiKey),
		http:    &http.Client{Timeout: timeout},
		upload:  &http.Client{Timeout: uploadTimeout},
	}
}

// APIError giữ lỗi an toàn từ RAGFlow mà không chứa API key hoặc payload gốc.
type APIError struct {
	HTTPStatus int
	Code       int
	Message    string
	Retryable  bool
}

func (e *APIError) Error() string {
	return fmt.Sprintf("RAGFlow lỗi: http=%d code=%d message=%s", e.HTTPStatus, e.Code, e.Message)
}

type envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func (c *Client) Health(ctx context.Context) error {
	req, err := c.request(ctx, http.MethodGet, "/api/v1/system/healthz", nil)
	if err != nil {
		return err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("gọi RAGFlow health: %w", err)
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
	if res.StatusCode/100 != 2 {
		return &APIError{HTTPStatus: res.StatusCode, Message: "health check thất bại", Retryable: retryableStatus(res.StatusCode)}
	}
	return nil
}

func (c *Client) CreateDataset(ctx context.Context, name, description string) (port.RAGDataset, error) {
	body := map[string]any{"name": name, "description": description, "permission": "me"}
	var remote struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/datasets", body, &remote); err != nil {
		return port.RAGDataset{}, err
	}
	if remote.ID == "" {
		return port.RAGDataset{}, errors.New("RAGFlow create dataset thiếu id")
	}
	return port.RAGDataset{ID: remote.ID, Name: remote.Name}, nil
}

func (c *Client) FindDatasetByName(ctx context.Context, name string) (*port.RAGDataset, error) {
	var env envelope
	endpoint := "/api/v1/datasets?page=1&page_size=30&name=" + url.QueryEscape(name)
	if err := c.doEnvelope(ctx, http.MethodGet, endpoint, nil, &env); err != nil {
		// RAGFlow trả codeNotOwned cho MỌI tên dataset user không sở hữu, kể cả
		// tên chưa từng tồn tại. Với caller thì cả hai đều là "không tìm thấy",
		// nên phải trả nil để nhánh tạo mới chạy được — giữ nguyên lỗi ở đây thì
		// KHÔNG project nào tạo được dataset lần đầu.
		//
		// Endpoint datasets không nhận tham số keywords (trả code=101) nên không
		// dùng được cách đã xử lý ở FindDocumentByName.
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.Code == codeNotOwned {
			return nil, nil
		}
		return nil, err
	}
	var rows []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := decodeCollection(env.Data, []string{"datasets"}, &rows); err != nil {
		return nil, fmt.Errorf("decode danh sách dataset RAGFlow: %w", err)
	}
	for _, row := range rows {
		if strings.EqualFold(row.Name, name) {
			return &port.RAGDataset{ID: row.ID, Name: row.Name}, nil
		}
	}
	return nil, nil
}

func (c *Client) UpdateDataset(ctx context.Context, datasetID, name, description string) error {
	endpoint := fmt.Sprintf("/api/v1/datasets/%s", url.PathEscape(datasetID))
	return c.doJSON(ctx, http.MethodPut, endpoint, map[string]any{"name": name, "description": description}, nil)
}

func (c *Client) DeleteDatasets(ctx context.Context, datasetIDs []string) error {
	return c.doJSON(ctx, http.MethodDelete, "/api/v1/datasets", map[string]any{"ids": datasetIDs}, nil)
}

func (c *Client) UploadDocument(ctx context.Context, datasetID string, file port.RAGDocumentFile) (port.RAGDocument, error) {
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	writeDone := make(chan error, 1)
	go func() {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, escapeFilename(file.Name)))
		if file.ContentType != "" {
			header.Set("Content-Type", file.ContentType)
		}
		part, err := mw.CreatePart(header)
		if err == nil {
			_, err = io.Copy(part, file.Reader)
		}
		if closeErr := mw.Close(); err == nil {
			err = closeErr
		}
		_ = pw.CloseWithError(err)
		writeDone <- err
	}()

	endpoint := fmt.Sprintf("/api/v1/datasets/%s/documents", url.PathEscape(datasetID))
	req, err := c.request(ctx, http.MethodPost, endpoint, pr)
	if err != nil {
		_ = pr.Close()
		return port.RAGDocument{}, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	res, callErr := c.upload.Do(req)
	writeErr := <-writeDone
	if callErr != nil {
		return port.RAGDocument{}, fmt.Errorf("upload tài liệu lên RAGFlow: %w", callErr)
	}
	defer res.Body.Close()
	if writeErr != nil {
		return port.RAGDocument{}, fmt.Errorf("đọc file để upload RAGFlow: %w", writeErr)
	}
	env, err := decodeHTTPResponse(res)
	if err != nil {
		return port.RAGDocument{}, err
	}
	var rows []remoteDocument
	if err = decodeCollection(env.Data, []string{"docs", "documents"}, &rows); err != nil {
		return port.RAGDocument{}, fmt.Errorf("decode upload RAGFlow: %w", err)
	}
	if len(rows) != 1 || rows[0].ID == "" {
		return port.RAGDocument{}, fmt.Errorf("RAGFlow upload trả %d document, cần 1", len(rows))
	}
	return rows[0].toPort(), nil
}

func (c *Client) GetDocument(ctx context.Context, datasetID, documentID string) (port.RAGDocument, error) {
	var env envelope
	endpoint := fmt.Sprintf("/api/v1/datasets/%s/documents?page=1&page_size=1&id=%s",
		url.PathEscape(datasetID), url.QueryEscape(documentID))
	if err := c.doEnvelope(ctx, http.MethodGet, endpoint, nil, &env); err != nil {
		return port.RAGDocument{}, err
	}
	var rows []remoteDocument
	if err := decodeCollection(env.Data, []string{"docs", "documents"}, &rows); err != nil {
		return port.RAGDocument{}, fmt.Errorf("decode document RAGFlow: %w", err)
	}
	for _, row := range rows {
		if row.ID == documentID {
			return row.toPort(), nil
		}
	}
	return port.RAGDocument{}, &APIError{HTTPStatus: http.StatusNotFound, Message: "không tìm thấy document", Retryable: false}
}

func (c *Client) FindDocumentByName(ctx context.Context, datasetID, name string) (*port.RAGDocument, error) {
	var env envelope
	// RAGFlow trả code=102 "you don't own the document" khi filter name không
	// khớp document nào, kể cả dataset hợp lệ nhưng đang rỗng. Dùng keywords để
	// nhận collection rỗng bình thường, sau đó kiểm tra exact name tại local.
	endpoint := fmt.Sprintf("/api/v1/datasets/%s/documents?page=1&page_size=100&keywords=%s",
		url.PathEscape(datasetID), url.QueryEscape(name))
	if err := c.doEnvelope(ctx, http.MethodGet, endpoint, nil, &env); err != nil {
		return nil, err
	}
	var rows []remoteDocument
	if err := decodeCollection(env.Data, []string{"docs", "documents"}, &rows); err != nil {
		return nil, fmt.Errorf("decode document RAGFlow: %w", err)
	}
	for _, row := range rows {
		if row.Name == name {
			document := row.toPort()
			return &document, nil
		}
	}
	return nil, nil
}

func (c *Client) StartParsing(ctx context.Context, datasetID string, documentIDs []string) error {
	endpoint := fmt.Sprintf("/api/v1/datasets/%s/chunks", url.PathEscape(datasetID))
	return c.doJSON(ctx, http.MethodPost, endpoint, map[string]any{"document_ids": documentIDs}, nil)
}

func (c *Client) StopParsing(ctx context.Context, datasetID string, documentIDs []string) error {
	endpoint := fmt.Sprintf("/api/v1/datasets/%s/chunks", url.PathEscape(datasetID))
	return c.doJSON(ctx, http.MethodDelete, endpoint, map[string]any{"document_ids": documentIDs}, nil)
}

func (c *Client) DeleteDocuments(ctx context.Context, datasetID string, documentIDs []string) error {
	endpoint := fmt.Sprintf("/api/v1/datasets/%s/documents", url.PathEscape(datasetID))
	return c.doJSON(ctx, http.MethodDelete, endpoint, map[string]any{"ids": documentIDs}, nil)
}

func (c *Client) UpdateDocumentMetadata(
	ctx context.Context, datasetID string, documentIDs []string, metadata map[string]string,
) error {
	updates := make([]map[string]string, 0, len(metadata))
	keys := make([]string, 0, len(metadata))
	for key := range metadata {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		updates = append(updates, map[string]string{"key": key, "value": metadata[key]})
	}
	endpoint := fmt.Sprintf("/api/v1/datasets/%s/metadata/update", url.PathEscape(datasetID))
	return c.doJSON(ctx, http.MethodPost, endpoint, map[string]any{
		"selector": map[string]any{"document_ids": documentIDs}, "updates": updates,
	}, nil)
}

func (c *Client) Retrieve(ctx context.Context, in port.RAGRetrievalRequest) (port.RAGRetrievalResult, error) {
	body := map[string]any{
		"question": in.Question, "dataset_ids": in.DatasetIDs, "document_ids": in.DocumentIDs,
		"page": in.Page, "page_size": in.PageSize,
		"similarity_threshold":     in.SimilarityThreshold,
		"vector_similarity_weight": in.VectorSimilarityWeight, "keyword": in.Keyword,
	}
	var data struct {
		Chunks []struct {
			ID               string  `json:"id"`
			DatasetID        string  `json:"dataset_id"`
			DocumentID       string  `json:"document_id"`
			DocumentName     string  `json:"document_keyword"`
			Content          string  `json:"content"`
			Similarity       float64 `json:"similarity"`
			VectorSimilarity float64 `json:"vector_similarity"`
			TermSimilarity   float64 `json:"term_similarity"`
		} `json:"chunks"`
		Total int `json:"total"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/retrieval", body, &data); err != nil {
		return port.RAGRetrievalResult{}, err
	}
	out := port.RAGRetrievalResult{Total: data.Total, Chunks: make([]port.RAGChunk, len(data.Chunks))}
	for i, chunk := range data.Chunks {
		out.Chunks[i] = port.RAGChunk{
			ID: chunk.ID, DatasetID: chunk.DatasetID, DocumentID: chunk.DocumentID,
			DocumentName: chunk.DocumentName, Content: chunk.Content,
			Similarity: chunk.Similarity, VectorSimilarity: chunk.VectorSimilarity,
			TermSimilarity: chunk.TermSimilarity,
		}
	}
	return out, nil
}

func (c *Client) CreateChat(ctx context.Context, name string, datasetIDs []string) (port.RAGChat, error) {
	var remote struct {
		ID         string   `json:"id"`
		Name       string   `json:"name"`
		DatasetIDs []string `json:"dataset_ids"`
	}
	body := map[string]any{"name": name, "dataset_ids": datasetIDs}
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/chats", body, &remote); err != nil {
		return port.RAGChat{}, err
	}
	if remote.ID == "" {
		return port.RAGChat{}, errors.New("RAGFlow create chat thiếu id")
	}
	return port.RAGChat{ID: remote.ID, Name: remote.Name, DatasetIDs: remote.DatasetIDs}, nil
}

func (c *Client) FindChatByName(ctx context.Context, name string) (*port.RAGChat, error) {
	var env envelope
	endpoint := "/api/v1/chats?page=1&page_size=2&name=" + url.QueryEscape(name)
	if err := c.doEnvelope(ctx, http.MethodGet, endpoint, nil, &env); err != nil {
		return nil, err
	}
	var rows []struct {
		ID         string   `json:"id"`
		Name       string   `json:"name"`
		DatasetIDs []string `json:"dataset_ids"`
	}
	if err := decodeCollection(env.Data, []string{"chats"}, &rows); err != nil {
		return nil, fmt.Errorf("decode danh sách chat RAGFlow: %w", err)
	}
	for _, row := range rows {
		if row.Name == name {
			return &port.RAGChat{ID: row.ID, Name: row.Name, DatasetIDs: row.DatasetIDs}, nil
		}
	}
	return nil, nil
}

func (c *Client) UpdateChatDatasets(ctx context.Context, chatID string, datasetIDs []string) error {
	endpoint := fmt.Sprintf("/api/v1/chats/%s", url.PathEscape(chatID))
	return c.doJSON(ctx, http.MethodPatch, endpoint, map[string]any{"dataset_ids": datasetIDs}, nil)
}

func (c *Client) CompleteChat(
	ctx context.Context, in port.RAGChatCompletionRequest,
) (port.RAGChatCompletionResult, error) {
	messages := make([]map[string]string, len(in.Messages))
	for i, message := range in.Messages {
		messages[i] = map[string]string{"role": message.Role, "content": message.Content}
	}
	extra := map[string]any{"reference": true}
	if len(in.MetadataConditions) > 0 {
		conditions := make([]map[string]string, len(in.MetadataConditions))
		for i, condition := range in.MetadataConditions {
			conditions[i] = map[string]string{
				"name": condition.Name, "comparison_operator": condition.Operator, "value": condition.Value,
			}
		}
		logic := in.MetadataLogic
		if logic == "" {
			logic = "and"
		}
		extra["metadata_condition"] = map[string]any{"logic": logic, "conditions": conditions}
	}
	body := map[string]any{
		"model": "model", "messages": messages, "stream": false, "extra_body": extra,
	}
	endpoint := fmt.Sprintf("/api/v1/openai/%s/chat/completions", url.PathEscape(in.ChatID))
	result, err := c.completeChatAt(ctx, endpoint, body)
	if err == nil {
		return result, nil
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.HTTPStatus != http.StatusNotFound {
		return port.RAGChatCompletionResult{}, err
	}
	legacy := fmt.Sprintf("/api/v1/chats_openai/%s/chat/completions", url.PathEscape(in.ChatID))
	return c.completeChatAt(ctx, legacy, body)
}

func (c *Client) completeChatAt(
	ctx context.Context, endpoint string, body any,
) (port.RAGChatCompletionResult, error) {
	rawBody, err := json.Marshal(body)
	if err != nil {
		return port.RAGChatCompletionResult{}, fmt.Errorf("encode RAGFlow chat request: %w", err)
	}
	req, err := c.request(ctx, http.MethodPost, endpoint, bytes.NewReader(rawBody))
	if err != nil {
		return port.RAGChatCompletionResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return port.RAGChatCompletionResult{}, fmt.Errorf("gọi RAGFlow POST %s: %w", endpoint, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
	if err != nil {
		return port.RAGChatCompletionResult{}, fmt.Errorf("đọc RAGFlow chat response: %w", err)
	}
	if len(raw) > maxResponseBytes {
		return port.RAGChatCompletionResult{}, errors.New("RAGFlow chat response vượt giới hạn")
	}
	if res.StatusCode/100 != 2 {
		return port.RAGChatCompletionResult{}, decodeAPIError(res.StatusCode, raw)
	}
	var response struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content   string          `json:"content"`
				Reference json.RawMessage `json:"reference"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err = json.Unmarshal(raw, &response); err != nil {
		return port.RAGChatCompletionResult{}, fmt.Errorf("decode RAGFlow chat response: %w", err)
	}
	if response.Code != 0 {
		return port.RAGChatCompletionResult{}, &APIError{
			HTTPStatus: http.StatusBadGateway, Code: response.Code,
			Message: response.Message, Retryable: false,
		}
	}
	if len(response.Choices) == 0 || strings.TrimSpace(response.Choices[0].Message.Content) == "" {
		return port.RAGChatCompletionResult{}, errors.New("RAGFlow chat không trả nội dung")
	}
	references, err := decodeChatReferences(response.Choices[0].Message.Reference)
	if err != nil {
		return port.RAGChatCompletionResult{}, fmt.Errorf("decode RAGFlow chat references: %w", err)
	}
	return port.RAGChatCompletionResult{
		Content: response.Choices[0].Message.Content, Model: response.Model, References: references,
	}, nil
}

// decodeChatReferences hỗ trợ cả hai schema mà các phiên bản RAGFlow đang trả:
// reference là mảng chunk trực tiếp, hoặc object chứa field chunks.
func decodeChatReferences(raw json.RawMessage) ([]port.RAGChunk, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return []port.RAGChunk{}, nil
	}
	if trimmed[0] == '[' {
		return decodeReferenceChunks(trimmed)
	}
	var reference struct {
		Chunks json.RawMessage `json:"chunks"`
	}
	if err := json.Unmarshal(trimmed, &reference); err != nil {
		return nil, err
	}
	return decodeReferenceChunks(reference.Chunks)
}

func decodeReferenceChunks(raw json.RawMessage) ([]port.RAGChunk, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return []port.RAGChunk{}, nil
	}
	type chunk struct {
		ID, DatasetID, DocumentID, DocumentName, Content string
		Similarity, VectorSimilarity, TermSimilarity     float64
	}
	decode := func(value json.RawMessage) (chunk, error) {
		var remote struct {
			ID               string  `json:"id"`
			DatasetID        string  `json:"dataset_id"`
			DocumentID       string  `json:"document_id"`
			DocumentName     string  `json:"document_name"`
			Content          string  `json:"content"`
			ContentWeighted  string  `json:"content_with_weight"`
			Similarity       float64 `json:"similarity"`
			VectorSimilarity float64 `json:"vector_similarity"`
			TermSimilarity   float64 `json:"term_similarity"`
		}
		if err := json.Unmarshal(value, &remote); err != nil {
			return chunk{}, err
		}
		content := remote.Content
		if content == "" {
			content = remote.ContentWeighted
		}
		return chunk{
			ID: remote.ID, DatasetID: remote.DatasetID, DocumentID: remote.DocumentID,
			DocumentName: remote.DocumentName, Content: content, Similarity: remote.Similarity,
			VectorSimilarity: remote.VectorSimilarity, TermSimilarity: remote.TermSimilarity,
		}, nil
	}
	values := make([]json.RawMessage, 0)
	trimmed := bytes.TrimSpace(raw)
	if trimmed[0] == '[' {
		if err := json.Unmarshal(trimmed, &values); err != nil {
			return nil, err
		}
	} else {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &object); err != nil {
			return nil, err
		}
		keys := make([]string, 0, len(object))
		for key := range object {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i, j int) bool {
			left, leftErr := strconv.Atoi(keys[i])
			right, rightErr := strconv.Atoi(keys[j])
			if leftErr == nil && rightErr == nil {
				return left < right
			}
			return keys[i] < keys[j]
		})
		for _, key := range keys {
			values = append(values, object[key])
		}
	}
	out := make([]port.RAGChunk, 0, len(values))
	for _, value := range values {
		item, err := decode(value)
		if err != nil {
			return nil, err
		}
		out = append(out, port.RAGChunk{
			ID: item.ID, DatasetID: item.DatasetID, DocumentID: item.DocumentID,
			DocumentName: item.DocumentName, Content: item.Content, Similarity: item.Similarity,
			VectorSimilarity: item.VectorSimilarity, TermSimilarity: item.TermSimilarity,
		})
	}
	return out, nil
}

func decodeAPIError(status int, raw []byte) error {
	var env envelope
	_ = json.Unmarshal(raw, &env)
	message := strings.TrimSpace(env.Message)
	if message == "" {
		message = strings.TrimSpace(string(raw))
	}
	if message == "" {
		message = http.StatusText(status)
	}
	return &APIError{HTTPStatus: status, Code: env.Code, Message: message, Retryable: retryableStatus(status)}
}

func (c *Client) doJSON(ctx context.Context, method, endpoint string, body, out any) error {
	var env envelope
	if err := c.doEnvelope(ctx, method, endpoint, body, &env); err != nil {
		return err
	}
	if out == nil || len(env.Data) == 0 || bytes.Equal(env.Data, []byte("null")) {
		return nil
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return fmt.Errorf("decode RAGFlow response data: %w", err)
	}
	return nil
}

func (c *Client) doEnvelope(ctx context.Context, method, endpoint string, body any, out *envelope) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode RAGFlow request: %w", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := c.request(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("gọi RAGFlow %s %s: %w", method, endpoint, err)
	}
	defer res.Body.Close()
	env, err := decodeHTTPResponse(res)
	if err != nil {
		return err
	}
	*out = env
	return nil
}

func (c *Client) request(ctx context.Context, method, endpoint string, body io.Reader) (*http.Request, error) {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("ragflow base URL không hợp lệ: %w", err)
	}
	u.Path = path.Join(u.Path, endpoint)
	// path.Join giữ query ngoài Path; endpoint nội bộ đã encode query riêng.
	if i := strings.IndexByte(endpoint, '?'); i >= 0 {
		u.Path = path.Join(strings.TrimRight(urlPathOnly(c.baseURL), "/"), endpoint[:i])
		u.RawQuery = endpoint[i+1:]
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, fmt.Errorf("tạo RAGFlow request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	return req, nil
}

func decodeHTTPResponse(res *http.Response) (envelope, error) {
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
	if err != nil {
		return envelope{}, fmt.Errorf("đọc RAGFlow response: %w", err)
	}
	if len(raw) > maxResponseBytes {
		return envelope{}, errors.New("RAGFlow response vượt giới hạn")
	}
	var env envelope
	if len(raw) > 0 {
		if err = json.Unmarshal(raw, &env); err != nil {
			return envelope{}, &APIError{HTTPStatus: res.StatusCode, Message: "response JSON không hợp lệ", Retryable: retryableStatus(res.StatusCode)}
		}
	}
	if res.StatusCode/100 != 2 || env.Code != 0 {
		message := strings.TrimSpace(env.Message)
		if message == "" {
			message = http.StatusText(res.StatusCode)
		}
		return envelope{}, &APIError{HTTPStatus: res.StatusCode, Code: env.Code, Message: message, Retryable: retryableStatus(res.StatusCode)}
	}
	return env, nil
}

func decodeCollection(raw json.RawMessage, keys []string, out any) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil
	}
	if raw[0] == '[' {
		return json.Unmarshal(raw, out)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return err
	}
	for _, key := range keys {
		if value, ok := object[key]; ok {
			return json.Unmarshal(value, out)
		}
	}
	return errors.New("response không chứa collection mong đợi")
}

type scalarString string

func (s *scalarString) UnmarshalJSON(raw []byte) error {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		*s = ""
		return nil
	}
	if raw[0] == '"' {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return err
		}
		*s = scalarString(strings.ToUpper(value))
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err != nil {
		return err
	}
	value, err := strconv.ParseFloat(number.String(), 64)
	if err != nil {
		return err
	}
	*s = scalarString(strconv.FormatInt(int64(value), 10))
	return nil
}

type remoteDocument struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Run         scalarString `json:"run"`
	Progress    float64      `json:"progress"`
	ProgressMsg string       `json:"progress_msg"`
}

func (d remoteDocument) toPort() port.RAGDocument {
	return port.RAGDocument{ID: d.ID, Name: d.Name, Run: string(d.Run), Progress: d.Progress, ProgressMsg: d.ProgressMsg}
}

func retryableStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500
}

func escapeFilename(name string) string {
	name = strings.ReplaceAll(name, `\`, "_")
	return strings.ReplaceAll(name, `"`, "_")
}

func urlPathOnly(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Path
}
