-- Cho phép người dùng loại 1 edge case khỏi phụ lục URD sinh ra khi hướng
-- giải quyết thực chất là "không áp dụng"/"chưa có chức năng" (không phải
-- một quyết định nghiệp vụ cần đưa vào tài liệu). Mặc định true để giữ hành
-- vi cũ (mọi case có hướng giải quyết đều được đưa vào tài liệu) cho các bản
-- ghi đã tồn tại và cho client chưa gửi trường này.
ALTER TABLE urd_edge_cases ADD COLUMN IF NOT EXISTS include_in_document BOOLEAN NOT NULL DEFAULT true;
