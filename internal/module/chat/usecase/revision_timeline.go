package usecase

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	retrievaldomain "github.com/quangdung93/docs-hub-api/internal/module/retrieval/domain"
)

// uploadTimeOffset là giờ Việt Nam (UTC+7) để hiển thị thời điểm upload cho AI;
// dùng FixedZone vì image distroless không có tzdata cho LoadLocation.
const uploadTimeOffset = 7 * 60 * 60

// revisionOrdinal là vị trí của 1 revision trong các bản của CÙNG tài liệu
// trong CÙNG scope: bản thứ n trên tổng số total, n tăng theo revision_no.
type revisionOrdinal struct {
	n, total int
}

// revisionKey gom các revision cùng tài liệu, cùng scope.
type revisionKey struct {
	scopeID, documentID uuid.UUID
}

// revisionOrdinals đánh số bản cho tài liệu có TỪ 2 revision trở lên trong
// cùng scope — vd URD ở v1.1 có bản gốc và bản đã hợp nhất edge case. Map
// theo RAGFlowDocumentID vì mỗi revision là 1 document riêng trên RAGFlow và
// chunk truy hồi được chỉ mang ID đó. Tài liệu chỉ có 1 bản không có mặt.
func revisionOrdinals(refs []retrievaldomain.RevisionRef) map[string]revisionOrdinal {
	groups := make(map[revisionKey][]retrievaldomain.RevisionRef)
	for _, ref := range refs {
		key := revisionKey{scopeID: ref.Scope.ID, documentID: ref.DocumentID}
		groups[key] = append(groups[key], ref)
	}
	out := make(map[string]revisionOrdinal)
	for _, group := range groups {
		if len(group) < 2 {
			continue
		}
		sort.SliceStable(group, func(i, j int) bool { return group[i].RevisionNo < group[j].RevisionNo })
		for i, ref := range group {
			out[ref.RAGFlowDocumentID] = revisionOrdinal{n: i + 1, total: len(group)}
		}
	}
	return out
}

// revisionLabel là nhãn hiển thị cho AI của 1 revision: nhãn scope, kèm số
// bản và thời điểm upload khi tài liệu có nhiều bản trong scope đó.
func revisionLabel(ref retrievaldomain.RevisionRef, ordinals map[string]revisionOrdinal) string {
	ordinal, ok := ordinals[ref.RAGFlowDocumentID]
	if !ok {
		return ref.Scope.Label
	}
	label := fmt.Sprintf("%s · bản %d/%d", ref.Scope.Label, ordinal.n, ordinal.total)
	if ordinal.n == ordinal.total {
		label += ", mới nhất"
	}
	if !ref.CreatedAt.IsZero() {
		label += ", upload " + ref.CreatedAt.In(time.FixedZone("UTC+7", uploadTimeOffset)).Format("02/01/2006 15:04")
	}
	return label
}

// revisionCatalog liệt kê các tài liệu có nhiều bản trong cùng scope để AI
// biết có thể (và cần) so sánh giữa các bản đó, không chỉ giữa các version.
func revisionCatalog(refs []retrievaldomain.RevisionRef, ordinals map[string]revisionOrdinal) string {
	if len(ordinals) == 0 {
		return ""
	}
	var lines []string
	seen := make(map[revisionKey]bool)
	for _, ref := range refs {
		key := revisionKey{scopeID: ref.Scope.ID, documentID: ref.DocumentID}
		if _, multi := ordinals[ref.RAGFlowDocumentID]; !multi || seen[key] {
			continue
		}
		seen[key] = true
		var versions []string
		for _, other := range sortedRevisions(refs, key) {
			versions = append(versions, revisionLabel(other, ordinals))
		}
		name := ref.Title
		if name == "" {
			name = ref.FileName
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", name, strings.Join(versions, "; ")))
	}
	return "\nTài liệu có nhiều bản trong cùng scope (bản số lớn hơn là bản cập nhật sau):\n" +
		strings.Join(lines, "\n")
}

func sortedRevisions(refs []retrievaldomain.RevisionRef, key revisionKey) []retrievaldomain.RevisionRef {
	var out []retrievaldomain.RevisionRef
	for _, ref := range refs {
		if ref.Scope.ID == key.scopeID && ref.DocumentID == key.documentID {
			out = append(out, ref)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].RevisionNo < out[j].RevisionNo })
	return out
}

// evidenceUnit là 1 mốc trên dòng thời gian khi lấy bằng chứng so sánh:
// 1 scope, hoặc 1 bản trong scope nếu tài liệu có nhiều bản ở scope đó.
type evidenceUnit struct {
	scopeID   uuid.UUID
	remoteIDs []string
}

// evidenceUnits chia refs thành các mốc theo thứ tự scope cũ đến mới. Scope
// có tài liệu nhiều bản được tách thành "bản 1", "bản 2"…: mỗi mốc gồm bản
// thứ k của các tài liệu nhiều bản; tài liệu chỉ có 1 bản đi cùng mốc đầu.
// Nhờ vậy truy hồi riêng từng bản, bản cũ không bị bản mới lấn hết chỗ.
func evidenceUnits(
	scopes []retrievaldomain.ResolvedScope, refs []retrievaldomain.RevisionRef,
	ordinals map[string]revisionOrdinal,
) []evidenceUnit {
	var units []evidenceUnit
	for _, scope := range scopes {
		buckets := map[int][]string{}
		maxBucket := 0
		for _, ref := range refs {
			if ref.Scope.ID != scope.ID || ref.RAGFlowDocumentID == "" {
				continue
			}
			bucket := 1
			if ordinal, ok := ordinals[ref.RAGFlowDocumentID]; ok {
				bucket = ordinal.n
			}
			buckets[bucket] = append(buckets[bucket], ref.RAGFlowDocumentID)
			maxBucket = max(maxBucket, bucket)
		}
		for bucket := 1; bucket <= maxBucket; bucket++ {
			if len(buckets[bucket]) > 0 {
				units = append(units, evidenceUnit{scopeID: scope.ID, remoteIDs: buckets[bucket]})
			}
		}
	}
	return units
}
