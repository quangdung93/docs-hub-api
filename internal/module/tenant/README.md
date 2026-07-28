# Module `tenant` (scaffold)

Chưa hiện thực. Quản lý người thuê (tenant) cho kiến trúc multi-tenant.

## Quyết định cần chốt SỚM
Nếu sản phẩm hướng multi-tenant, **thêm `tenant_id` vào MỌI migration ngay từ đầu**. Retrofit sau khi đã có dữ liệu là migration đau đớn và rủi ro.

## Hướng hiện thực gợi ý
- Middleware phân giải tenant từ subdomain/header/JWT claim, đặt `tenant_id` vào context.
- GORM scope tự động thêm `WHERE tenant_id = ?` cho mọi truy vấn (dùng callback plugin).

## Trách nhiệm
CRUD tenant, cấu hình theo tenant, giới hạn tài nguyên (quota).

## Port cần
- `domain.TenantRepository` (mới).
- Tận dụng `port.Cache` để cache cấu hình tenant.
