-- Thêm cột avatar_key vào bảng projects: object key trong MinIO chứa ảnh đại
-- diện dự án. NULL nghĩa là chưa có ảnh (hoặc đã xin presigned URL nhưng CHƯA
-- xác nhận upload xong) — chỉ ghi cột này SAU KHI usecase xác nhận object đã
-- thực sự tồn tại trong storage (xem domain.Project.SetAvatar).
ALTER TABLE projects ADD COLUMN IF NOT EXISTS avatar_key TEXT;