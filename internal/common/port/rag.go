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
	Retrieve(ctx context.Context, input RAGRetrievalRequest) (RAGRetrievalResult, error)
}
