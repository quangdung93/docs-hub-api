-- Nhả sha256 của những tài liệu bị xoá TRƯỚC khi có bản sửa ở SoftDelete.
--
-- Hai chỉ số uk_revisions_version_hash và uk_revisions_change_hash
-- (migrations/000007) là chỉ số RIÊNG PHẦN, loại trừ status='archived'. Từ
-- trước tới nay KHÔNG có đoạn mã nào ghi giá trị đó, nên revision của tài liệu
-- đã xoá vẫn nằm trong chỉ số và tiếp tục khoá nội dung: người dùng xoá tài
-- liệu rồi tải lại đúng file đó lên vẫn bị báo "đã tồn tại trong phạm vi".
--
-- Câu này chỉ BỚT dòng khỏi chỉ số, không bao giờ thêm vào, nên không thể gây
-- vi phạm ràng buộc. Chạy lại lần nữa cũng không đổi gì thêm.
--
-- Nó KHÔNG đụng documents.deleted_at, KHÔNG sinh outbox_events và KHÔNG sinh
-- ingestion_jobs — tài liệu vẫn ở trạng thái đã xoá, không có gì được nạp lại
-- lên RAGFlow. Worker nhận việc từ ingestion_jobs và outbox_events, chưa bao
-- giờ dò theo document_revisions.status.
UPDATE document_revisions r
SET status = 'archived', updated_at = now()
FROM documents d
WHERE d.id = r.document_id
  AND d.deleted_at IS NOT NULL
  AND r.status <> 'archived';
