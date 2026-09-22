-- Tiêu đề mục (AC/BR/...) mà AI đề xuất bổ sung nội dung xử lý edge case vào,
-- copy nguyên văn từ tài liệu URD. Dùng để chèn THẲNG nội dung vào đúng mục
-- trong phiên bản URD mới thay vì gom hết vào 1 phụ lục cuối tài liệu.
-- Rỗng = AI không xác định được mục nào → vẫn lùi về phụ lục như trước.
ALTER TABLE urd_edge_cases ADD COLUMN IF NOT EXISTS target_heading TEXT NOT NULL DEFAULT '';
