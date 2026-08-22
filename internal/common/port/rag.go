package port

import (
	"context"
	"io"
)

// RAGDocumentFile là file local được gửi sang RAG engine.
type RAGDocumentFile struct {
	Name        string
	ContentType string
	Reader      io.Reader
}

type RAGDataset struct {
	ID   string
	Name string
}

type RAGDocument struct {
	ID          string
	Name        string
	Run         string
	Progress    float64
	ProgressMsg string
}

func (d RAGDocument) Ready() bool {
	if d.Run != "" {
		return d.Run == "DONE" || d.Run == "3"
	}
	return d.Progress >= 1
}

func (d RAGDocument) Failed() bool {
	return d.Run == "FAIL" || d.Run == "4" || d.Run == "CANCEL" || d.Run == "2"
}

type RAGRetrievalRequest struct {
	Question               string
	DatasetIDs             []string
	DocumentIDs            []string
	Page                   int
	PageSize               int
	SimilarityThreshold    float64
	VectorSimilarityWeight float64
	Keyword                bool
}

type RAGChunk struct {
	ID               string
	DatasetID        string
	DocumentID       string
	DocumentName     string
	Content          string
	Similarity       float64
	VectorSimilarity float64
	TermSimilarity   float64
}

type RAGRetrievalResult struct {
	Chunks []RAGChunk
	Total  int
}

type RAGChat struct {
	ID         string
	Name       string
	DatasetIDs []string
}

type RAGChatMessage struct {
	Role    string
	Content string
}

type RAGMetadataCondition struct {
	Name     string
	Operator string
	Value    string
}

type RAGChatCompletionRequest struct {
	ChatID             string
	Messages           []RAGChatMessage
	MetadataLogic      string
	MetadataConditions []RAGMetadataCondition
}

type RAGChatCompletionResult struct {
	Content    string
	Model      string
	References []RAGChunk
}

// RAGClient là capability tối thiểu dùng chung cho ingestion và retrieval.
type RAGClient interface {
	Health(ctx context.Context) error
	CreateDataset(ctx context.Context, name, description string) (RAGDataset, error)
	FindDatasetByName(ctx context.Context, name string) (*RAGDataset, error)
	UpdateDataset(ctx context.Context, datasetID, name, description string) error
	DeleteDatasets(ctx context.Context, datasetIDs []string) error
	UploadDocument(ctx context.Context, datasetID string, file RAGDocumentFile) (RAGDocument, error)
	GetDocument(ctx context.Context, datasetID, documentID string) (RAGDocument, error)
	FindDocumentByName(ctx context.Context, datasetID, name string) (*RAGDocument, error)
	StartParsing(ctx context.Context, datasetID string, documentIDs []string) error
	StopParsing(ctx context.Context, datasetID string, documentIDs []string) error
	DeleteDocuments(ctx context.Context, datasetID string, documentIDs []string) error
	UpdateDocumentMetadata(ctx context.Context, datasetID string, documentIDs []string, metadata map[string]string) error
	Retrieve(ctx context.Context, input RAGRetrievalRequest) (RAGRetrievalResult, error)
	CreateChat(ctx context.Context, name string, datasetIDs []string) (RAGChat, error)
	FindChatByName(ctx context.Context, name string) (*RAGChat, error)
	UpdateChatDatasets(ctx context.Context, chatID string, datasetIDs []string) error
	CompleteChat(ctx context.Context, input RAGChatCompletionRequest) (RAGChatCompletionResult, error)
}
