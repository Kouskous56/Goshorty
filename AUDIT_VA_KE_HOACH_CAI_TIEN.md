# Báo cáo đánh giá chuyên sâu và kế hoạch cải tiến GoShorty

**Ngày đánh giá:** 28/07/2026  
**Nhánh được đánh giá:** `main`  
**Commit hiện tại:** `6a7a266` — `Fix short URL uses request Host instead of config BASE_URL`  
**Phạm vi:** toàn bộ mã Go, SPA frontend, cấu hình, test, tài liệu và lịch sử commit hiện có trong repository.

---

## 1. Mục tiêu và phương pháp đánh giá

Báo cáo này nhằm trả lời bốn câu hỏi:

1. GoShorty hiện đang hoạt động như thế nào?
2. Những phần nào đã được thiết kế và triển khai tốt?
3. Những lỗi, rủi ro hoặc giới hạn nào còn tồn tại?
4. Nên cải tiến dự án theo thứ tự nào để đạt chất lượng production?

Các nội dung đã được đọc và đối chiếu:

- Entry point và route registration trong `main.go`.
- Các package `config`, `models`, `storage`, `services`, `handlers`, `utils`.
- Toàn bộ JavaScript chính trong `static/index.html`.
- Test trong `main_test.go` và `handlers/auth_test.go`.
- Cấu hình Railway, Makefile và dependency.
- Tài liệu README, kiến trúc, API, kiểm thử, triển khai và các báo cáo sửa lỗi trước.
- Các commit gần nhất để xác định thay đổi mới chưa được phản ánh trong tài liệu.

### Trạng thái xác minh

Go toolchain không có trong `PATH`, nhưng đã được tìm thấy tại `C:\Program Files\Go\bin` và được gọi bằng đường dẫn tuyệt đối.

Sau khi hoàn thiện Giai đoạn 1:

- `gofmt` đã được chạy trên toàn bộ file Go thay đổi.
- `go test ./... -count=1` đã chạy thành công cho tất cả package.
- `go vet ./...` đã chạy thành công.
- Coverage hiện tại: `config` 57,9%; `handlers` 51,2%; `services` 31,1%; `storage` 36,6%; các package còn lại chưa có coverage đáng kể.
- `go test -race ./...` chưa chạy được vì Windows chưa có C compiler `gcc` cho CGO.

Do đó các regression test thông thường đã được xác minh động; race detector vẫn cần được chạy lại trong CI Linux hoặc sau khi cài C toolchain.

---

## 2. Tóm tắt điều hành

GoShorty là một URL shortener dạng monolith nhỏ, viết bằng Go 1.21 và Gin. Backend API, frontend SPA và tài nguyên tĩnh được đóng gói vào cùng một binary. Hệ thống hỗ trợ:

- Đăng ký và đăng nhập.
- Hai vai trò `admin` và `user`.
- Token ký bằng HMAC-SHA256.
- Mật khẩu băm bằng bcrypt.
- URL rút gọn có TTL.
- Custom short code.
- Theo dõi lượt truy cập.
- Phân quyền quản lý URL.
- Quản trị người dùng.
- Graceful shutdown và cleanup URL hết hạn.
- Deploy lên Railway.

### Kết luận ngắn

Đây là một dự án demo/học tập có chất lượng khá, cấu trúc dễ hiểu và đã có nhiều vòng hardening. Dự án chưa nên được coi là production-ready vì còn bốn nhóm rủi ro chính:

1. **Tính đúng đắn:** cách tạo public URL chưa an toàn sau reverse proxy; `timeout` trong path không được sử dụng; một số endpoint không áp ownership đầy đủ.
2. **Concurrency:** thao tác kiểm tra rồi ghi custom code không atomic; storage trả pointer nội bộ ra ngoài mutex.
3. **Authentication và authorization:** role trong token có thể cũ; user bị xóa vẫn dùng được token ở phần lớn endpoint.
4. **Vận hành:** toàn bộ dữ liệu nằm trong RAM; restart/redeploy làm mất dữ liệu; không thể scale nhiều instance một cách đúng đắn.

### Đánh giá theo nhóm

| Nhóm | Đánh giá | Nhận xét |
|---|---:|---|
| Cấu trúc mã | 7/10 | Phân lớp rõ, quy mô phù hợp, nhưng dependency wiring khó test |
| Tính năng | 7/10 | Đủ luồng cơ bản, một số semantics chưa nhất quán |
| Bảo mật | 5/10 | Có HMAC, bcrypt và role middleware nhưng thiếu revocation/rate limit |
| Concurrency | 5/10 | Có mutex nhưng vẫn tồn tại check-then-set và pointer escape |
| Test | 5/10 | Có test auth và URL cơ bản, thiếu integration/race/security |
| Frontend | 6/10 | Hoàn chỉnh cho demo, nhưng state/token và rendering cần hardening |
| Vận hành | 3/10 | In-memory, thiếu observability, migration và persistence |
| Tài liệu | 7/10 | Rất nhiều tài liệu, nhưng trùng lặp và lệch mã hiện tại |

---

## 3. Kiến trúc hiện tại

### 3.1 Sơ đồ thành phần

```text
Browser
  |
  | HTML/CSS/JavaScript + JSON API
  v
Gin Router
  |
  +-- CORS middleware
  +-- Auth middleware
  +-- Admin middleware
  |
  +-- AuthHandler -------- TokenService
  |        |
  |        +-------------- UserStorage (in-memory)
  |
  +-- Handler ------------ URLService
                              |
                              +-- Storage (in-memory + TTL cleanup)
```

### 3.2 Trách nhiệm từng package

#### `config`

`config/config.go` chịu trách nhiệm:

- Đọc port từ `PORT`.
- Đọc `BASE_URL`.
- Khai báo các TTL hợp lệ: `5m`, `15m`, `1h`, `24h`, `168h`.

Điểm tốt:

- Cấu hình TTL tập trung.
- Có fallback phù hợp cho local development.

Điểm hạn chế:

- Config chưa bao gồm đầy đủ các biến môi trường đang dùng trong `main.go`.
- Việc đọc env bị chia giữa `config.NewConfig()` và `main()`.
- Không có bước validate config tập trung.

#### `models`

`models/user.go` và `models/url.go` định nghĩa request/response và domain data.

Điểm tốt:

- Có Gin binding tags.
- Password bị loại khỏi JSON bằng `json:"-"`.
- Role dùng constants thay vì lặp magic string.

Điểm hạn chế:

- Domain model và API response model còn dùng chung ở một số endpoint.
- `URLData.CreatedBy` bị trả ra API dù user thường không nhất thiết cần biết ID nội bộ.
- `CreatedAt` của user dùng Unix timestamp, trong khi URL dùng `time.Time`; format thời gian không nhất quán.
- `ShortenRequest.ExpiresIn` được validator giới hạn, nhưng service lại âm thầm fallback cho giá trị không hợp lệ nếu được gọi trực tiếp.

#### `storage`

`storage/storage.go` lưu URL trong map và cleanup mỗi 30 giây.  
`storage/user.go` lưu user theo username và ID.

Điểm tốt:

- Dùng `sync.RWMutex`.
- `GetAndIncrement` gộp lấy URL và tăng visit trong một lock.
- URL hết hạn bị bỏ qua ngay cả trước khi cleanup chạy.
- Password được băm trước khi lưu.

Điểm hạn chế:

- Không có persistence.
- Trả pointer nội bộ ra khỏi storage.
- Custom code chưa có `SetIfAbsent`.
- Không có canonicalization cho username/email.
- Không kiểm tra email trùng.
- Không có invariant bảo vệ admin cuối cùng.

#### `services`

`services/url_service.go` chứa nghiệp vụ URL.  
`services/token_service.go` sinh và xác minh token.

Điểm tốt:

- URL scheme được giới hạn ở HTTP/HTTPS.
- Ownership được áp dụng khi list và delete.
- Chữ ký token được so sánh bằng `hmac.Equal`.
- Token có thời gian phát hành và hết hạn.

Điểm hạn chế:

- Role được tin trực tiếp từ token.
- Token format là thiết kế riêng, chưa có version, issuer hoặc audience.
- Global mutable variable `TokenTTL` làm test song song và cấu hình khó kiểm soát.
- Service vẫn dựng `ShortURL`, sau đó handler lại ghi đè `ShortURL`; trách nhiệm bị trùng.

#### `handlers`

Handlers thực hiện JSON binding, lấy context auth và ánh xạ lỗi sang HTTP.

Điểm tốt:

- Endpoint được nhóm public, protected và admin.
- Error response có cấu trúc `message` và `code`.
- Login không phân biệt sai username hay password.
- Delete URL cố tình trả cùng một dạng 404 để hạn chế existence leak.

Điểm hạn chế:

- Mapping lỗi chủ yếu dựa trên mọi lỗi thành 400/404, chưa có error taxonomy rõ ràng.
- Validation error nội bộ được nối trực tiếp vào response.
- `GetURLInfo` chưa kiểm tra ownership.
- `GetStats` chưa lọc theo user.
- `AuthMiddleware` không kiểm tra user còn tồn tại hoặc role hiện hành.

#### `static/index.html`

Frontend là SPA thuần HTML/CSS/JavaScript.

Điểm tốt:

- Không cần build frontend riêng.
- API base dùng relative path `/api`.
- Các giá trị chính được escape trước khi ghép vào `innerHTML`.
- Error/success message đã được tách giữa auth và dashboard.
- Có null guard khi đọc user từ local storage.

Điểm hạn chế:

- Token lưu trong `localStorage`.
- Khi reload, frontend tin user object cũ thay vì gọi `/api/auth/me`.
- Có nhiều inline `onclick`, gây khó áp dụng CSP nghiêm ngặt.
- Một file hơn 1.000 dòng chứa cả HTML, CSS và JS.
- Có dấu hiệu lỗi encoding emoji trong source.
- Không có frontend test hoặc browser test.

---

## 4. Bản đồ endpoint và quyền truy cập

| Method | Endpoint | Quyền | Chức năng |
|---|---|---|---|
| GET | `/health` | Public | Health check |
| GET | `/api` | Public | Thông tin API |
| POST | `/api/auth/login` | Public | Đăng nhập |
| POST | `/api/auth/register` | Public | Đăng ký |
| GET | `/goshorty/:timeout/:code` | Public | Redirect |
| GET | `/api/auth/me` | Authenticated | User hiện tại |
| POST | `/api/shorten` | Authenticated | Tạo URL |
| GET | `/api/shorten/:code` | Authenticated | Xem URL info |
| DELETE | `/api/shorten/:code` | Owner/Admin | Xóa URL |
| GET | `/api/shorten/all` | Authenticated | List theo role |
| GET | `/api/stats` | Authenticated | Số liệu toàn hệ thống hiện tại |
| GET | `/api/auth/users` | Admin | Danh sách user |
| PUT | `/api/auth/users/:username/role` | Admin | Đổi role |
| DELETE | `/api/auth/users/:username` | Admin | Xóa user |

### Nhận xét routing

- Route redirect chứa `:timeout`, nhưng handler chỉ đọc `:code`.
- `GET /api/shorten/:code` và `GET /api/shorten/all` dựa vào khả năng ưu tiên static route của router. Nên tránh thiết kế dễ nhầm bằng endpoint rõ hơn như `/api/urls` và `/api/urls/:code`.
- `/api` vừa là prefix route vừa là endpoint mô tả API; hợp lệ nhưng không phải cách tổ chức rõ nhất.

---

## 5. Audit luồng nghiệp vụ

### 5.1 Đăng ký

Luồng:

1. Bind JSON và validate username/password/email.
2. Kiểm tra username tồn tại.
3. Băm password bằng bcrypt.
4. Tạo user role `user`.
5. Sinh token.
6. Trả token và user.

Rủi ro:

- Username phân biệt hoa thường, nên `Admin`, `admin` và `ADMIN` có thể là ba tài khoản.
- Email không được normalize hoặc kiểm tra trùng.
- Không giới hạn độ dài password tối đa, có thể tăng chi phí xử lý request lớn không cần thiết.
- Không rate limit.
- User đã được tạo trước khi sinh token; nếu sinh token lỗi, client nhận 500 nhưng user vẫn tồn tại.

### 5.2 Đăng nhập

Luồng:

1. Bind username/password.
2. Lấy user.
3. `bcrypt.CompareHashAndPassword`.
4. Sinh signed token.

Điểm tốt:

- Response sai credential thống nhất.

Rủi ro:

- Với username không tồn tại, hệ thống không chạy bcrypt; timing có thể khác username tồn tại.
- Không rate limit, cooldown hoặc audit log.

### 5.3 Xác thực token

Token có dạng:

```text
base64url(JSON claims).base64url(HMAC-SHA256 signature)
```

Claims gồm:

- `user_id`
- `username`
- `role`
- `issued_at`
- `expires_at`

Rủi ro:

- Role và username là snapshot tại lúc login.
- Không có token ID (`jti`) để revoke.
- Không có issuer/audience để tách môi trường hoặc dịch vụ.
- Không có key ID/version để rotate secret mượt mà.
- Không kiểm tra `IssuedAt` nằm hợp lý trong quá khứ.
- Nếu user bị xóa, token vẫn qua middleware.

### 5.4 Tạo URL rút gọn

Luồng:

1. Gin validator kiểm tra `url`, `expires_in`, `custom_code`.
2. Service parse URL và giới hạn scheme.
3. Chọn TTL.
4. Kiểm tra custom code hoặc sinh random code.
5. Ghi vào storage.
6. Dựng short URL.

Rủi ro:

- `Exists` rồi `Set` không atomic.
- Random code fallback 10 ký tự không được kiểm tra collision lần cuối.
- Host/scheme public URL được lấy từ request thiếu trust boundary.
- URL parse chưa kiểm tra hostname rỗng ở service một cách tường minh.
- Cho phép rút gọn URL quay lại chính dịch vụ, tạo redirect chain hoặc loop.
- Không chặn URL nội bộ; điều này hiện chưa thành SSRF vì server không fetch URL, nhưng cần lưu ý nếu sau này bổ sung preview.

### 5.5 Redirect

Luồng:

1. Lấy `code`.
2. `GetAndIncrement`.
3. Trả `302`.

Điểm tốt:

- Visit increment atomic với lookup.
- URL hết hạn bị từ chối ngay.

Rủi ro:

- `timeout` không được validate.
- Mọi request, kể cả bot, scanner và HEAD-like traffic qua GET, đều tăng visits.
- Không có click event chi tiết, IP anonymization hay user-agent analytics.
- Không có cache-control rõ ràng.

### 5.6 Quản lý URL

Điểm tốt:

- List URL lọc theo owner cho user thường.
- Delete kiểm tra owner hoặc admin.

Rủi ro:

- Get info không kiểm tra owner.
- Stats không lọc owner.
- Xóa user không xử lý URL thuộc user đó; URL trở thành orphan.
- Không có pagination hoặc sorting ổn định.
- Map iteration khiến thứ tự response thay đổi ngẫu nhiên.

### 5.7 Quản trị user

Rủi ro:

- Backend cho phép đổi role của tài khoản `admin`.
- Backend cho phép xóa tài khoản `admin`; frontend chỉ ẩn nút.
- Một admin có thể tự hạ quyền và khiến hệ thống không còn admin.
- Token cũ giữ role cũ sau khi role thay đổi.
- Không có audit trail ai đã đổi role hoặc xóa user.

---

## 6. Các phát hiện ưu tiên

### P0 — Phải xử lý trước khi public production

#### P0.1 Dữ liệu không bền vững

**Bằng chứng:** `storage/storage.go`, `storage/user.go` dùng map trong process.

**Tác động:**

- Restart/redeploy mất toàn bộ URL và user.
- Nhiều instance có dữ liệu khác nhau.
- Link đã chia sẻ có thể chết sau deploy.

**Cải tiến dự định:**

- Dùng PostgreSQL.
- Tạo repository interfaces.
- Thêm migration cho `users`, `urls`, có thể thêm `sessions` hoặc `token_versions`.
- Index unique cho username và short code.
- TTL được thực hiện bằng query `expires_at` và background cleanup có kiểm soát.

**Tiêu chí hoàn thành:**

- Restart server không mất dữ liệu.
- Hai instance đọc/ghi cùng dữ liệu chính xác.
- Duplicate custom code bị database từ chối atomically.

#### P0.2 Public URL sai hoặc có thể bị ảnh hưởng bởi Host header

**Bằng chứng:** `handlers/handler.go:47-52`.

**Tác động:**

- Railway có thể trả URL `http://` thay vì `https://`.
- Client có thể gửi Host tùy ý và nhận short URL chứa host đó.
- `BASE_URL` bị handler ghi đè.

**Cải tiến dự định:**

- Chọn một chiến lược duy nhất:
  - Production dùng `PUBLIC_BASE_URL` bắt buộc; hoặc
  - Cấu hình trusted proxies và đọc forwarded headers an toàn.
- Chỉ service/public URL builder chịu trách nhiệm dựng URL.

**Tiêu chí hoàn thành:**

- Test cho local HTTP.
- Test cho HTTPS sau trusted reverse proxy.
- Host header không thể thay đổi domain public khi đã cấu hình.

#### P0.3 Role/token không phản ánh user hiện hành

**Bằng chứng:** `handlers/auth.go:193-206`.

**Tác động:**

- User bị hạ quyền vẫn có quyền admin.
- User bị xóa vẫn gọi được nhiều API.

**Cải tiến dự định:**

- Sau khi verify chữ ký, middleware lấy user hiện hành theo `user_id`.
- Context lấy username/role từ database, không lấy role từ token.
- Bổ sung `token_version` hoặc session table để revoke token.

**Tiêu chí hoàn thành:**

- Đổi role có hiệu lực với request kế tiếp.
- Xóa/disable user vô hiệu hóa token ngay.
- Có test cho stale admin token.

#### P0.4 Race khi tạo custom/random code

**Bằng chứng:** `services/url_service.go:55-64`.

**Tác động:**

- Hai request đồng thời có thể cùng nhận một custom code.
- URL trước có thể bị ghi đè.

**Cải tiến dự định:**

- Ngắn hạn: `Storage.SetIfAbsent`.
- Dài hạn: unique constraint trong database và retry khi random collision.

**Tiêu chí hoàn thành:**

- Concurrency test với hàng trăm goroutine.
- Chỉ đúng một request custom code thành công.
- Không có overwrite.

### P1 — Bảo mật và tính đúng đắn cao

#### P1.1 Ownership chưa đầy đủ

Áp ownership cho:

- `GET /api/shorten/:code`.
- `GET /api/stats`.

Thiết kế dự định:

- User nhận info/stats của chính mình.
- Admin nhận toàn hệ thống.
- Response không trả `CreatedBy` cho user nếu không cần.

#### P1.2 Pointer nội bộ thoát khỏi mutex

Các hàm `Get`, `GetAll`, `GetAndIncrement` trả `*URLData` đang nằm trong map.

Cải tiến:

- Trả value/snapshot copy.
- Không để caller có khả năng đọc/ghi object nội bộ sau khi unlock.
- Chạy `go test -race ./...`.

#### P1.3 Admin mặc định không an toàn

`ADMIN_PASSWORD` fallback thành `admin123`.

Cải tiến:

- Production bắt buộc cung cấp password.
- Validate độ dài/độ mạnh cơ bản.
- Không ghi password ra log.
- Cân nhắc bootstrap admin bằng command riêng.

#### P1.4 Rate limiting

Ưu tiên bảo vệ:

1. Login.
2. Register.
3. Create short URL.
4. Redirect nếu bị abuse.

Cải tiến:

- Limit theo IP cho public auth.
- Limit theo user cho protected write.
- Trả `429 Too Many Requests`.
- Có header retry phù hợp.

#### P1.5 Error handling

Cải tiến:

- Định nghĩa sentinel/domain errors.
- Dùng `errors.Is`.
- Handler map rõ `400`, `401`, `403`, `404`, `409`, `429`, `500`.
- Không trả chi tiết validator hoặc lỗi nội bộ trong production.
- Duplicate custom code nên là `409 Conflict`.

#### P1.6 Bảo vệ admin invariants

Cải tiến:

- Không cho xóa admin cuối cùng.
- Không cho hạ role admin cuối cùng.
- Xác định có cho phép tự đổi role/xóa chính mình hay không.
- Ghi audit event cho hành động quản trị.

### P2 — Chất lượng, vận hành và trải nghiệm

#### P2.1 Config tập trung

Tạo config thống nhất gồm:

- Server port.
- Public base URL.
- Secret/key settings.
- Token TTL.
- Admin bootstrap.
- CORS origins.
- Trusted proxies.
- Database DSN.
- Log level.

Validate một lần khi startup và fail fast với thông báo rõ.

#### P2.2 Observability

Thêm:

- Structured logging.
- Request/correlation ID.
- Latency, status và route metrics.
- Counters cho created URLs, redirect hits, auth failures.
- `/health/live` và `/health/ready`.
- Không log password/token/full sensitive URL nếu không cần.

#### P2.3 API cleanup và versioning

Đề xuất:

```text
POST   /api/v1/auth/register
POST   /api/v1/auth/login
GET    /api/v1/auth/me
GET    /api/v1/urls
POST   /api/v1/urls
GET    /api/v1/urls/:code
DELETE /api/v1/urls/:code
GET    /api/v1/stats
GET    /r/:code
```

TTL không nên nằm trong redirect path vì expiry đã nằm trong dữ liệu.

#### P2.4 Pagination và sorting

- URL list sắp xếp theo `created_at DESC`.
- User list có thứ tự ổn định.
- Hỗ trợ `limit`, `cursor` hoặc page.
- Tránh trả toàn bộ dữ liệu trong một response.

#### P2.5 Frontend state và security

Cải tiến:

- Khi load, gọi `/auth/me` thay vì tin user object trong storage.
- Xử lý thống nhất response `401` bằng logout/session-expired.
- Tách JavaScript/CSS khỏi HTML.
- Thay inline handlers bằng `addEventListener`.
- Thêm CSP, `X-Content-Type-Options`, `Referrer-Policy`, frame policy.
- Nếu chuyển sang cookie: dùng `HttpOnly`, `Secure`, `SameSite` và CSRF protection.
- Nếu tiếp tục bearer token: giảm TTL và có refresh/session strategy phù hợp.

#### P2.6 Documentation consolidation

Giữ một bộ tài liệu chuẩn:

- `README.md`: tổng quan và quickstart.
- `docs/architecture.md`: kiến trúc hiện hành.
- `docs/api.md` hoặc OpenAPI.
- `docs/deployment.md`.
- `docs/security.md`.
- `CHANGELOG.md`.

Các báo cáo lịch sử như `baocao.md` và `1022.md` có thể chuyển vào `docs/history/`.

---

## 7. Kế hoạch triển khai dự kiến

### Trạng thái Giai đoạn 1 — Hoàn thành ngày 28/07/2026

Các hạng mục đã triển khai:

- Public short URL chỉ dùng `PUBLIC_BASE_URL`, có fallback tương thích sang `BASE_URL`; không còn tin trực tiếp `Host` của request.
- Public base URL được normalize và validate là URL tuyệt đối dùng HTTP/HTTPS.
- Storage có `SetIfAbsent` atomic; custom code không còn luồng `Exists` rồi `Set`.
- Random code reservation cũng dùng thao tác atomic và retry khi collision.
- URL và user storage trả snapshot copy thay vì pointer nội bộ.
- `Storage.Stop()` trở thành idempotent.
- Auth middleware tải user hiện hành ở mỗi request; xóa user hoặc đổi role có hiệu lực ngay.
- `GetURLInfo` và `GetStats` được giới hạn theo owner; admin vẫn có quyền toàn hệ thống.
- Không thể xóa hoặc hạ quyền admin cuối cùng.
- `ADMIN_PASSWORD` bắt buộc trong Gin release mode; development fallback có warning rõ.
- Bổ sung domain/sentinel errors và mapping HTTP rõ hơn.
- Duplicate username/custom code trả `409 Conflict`.
- Validation response không còn lộ chi tiết framework.
- Bổ sung regression tests cho atomic reservation, snapshot isolation, ownership, scoped stats, public base URL, stale role, deleted user token và last-admin invariant.

Phần còn giới hạn:

- Persistence thuộc Giai đoạn 2.
- Rate limiting, CORS/security headers và observability thuộc Giai đoạn 3.
- Race detector chưa chạy được do thiếu `gcc`; test concurrency thường đã chạy thành công.

### Trạng thái Giai đoạn 2 — Hoàn thành ngày 28/07/2026

Các hạng mục persistence đã triển khai:

- Thêm `URLStore` và `UserStore` interfaces; service/handler không còn phụ thuộc trực tiếp vào map storage.
- Thêm PostgreSQL implementation dùng `pgx/v5`, đồng thời giữ in-memory implementation cho local development.
- Release mode bắt buộc có `DATABASE_URL`; ứng dụng không còn âm thầm chạy bằng storage dễ mất dữ liệu trong production.
- Thêm migration ledger `schema_migrations` và migration runner có PostgreSQL advisory lock, cho phép nhiều instance khởi động an toàn.
- Thêm migration `001_init.sql` cho `users`, `urls`, unique constraints, foreign key và indexes.
- Short code được bảo vệ bằng unique constraint và atomic upsert; code của record đã hết hạn có thể được tái sử dụng.
- User deletion dùng `ON DELETE CASCADE` để không để lại URL orphan.
- Visit counter dùng một câu `UPDATE ... RETURNING`, bảo đảm atomic ở database.
- Last-admin invariant được bảo vệ trong transaction với advisory lock.
- Admin bootstrap idempotent bằng `ON CONFLICT DO NOTHING`.
- PostgreSQL queries trả thứ tự ổn định và lọc owner ngay tại database.
- Thêm background cleanup xóa URL hết hạn theo batch.
- Startup kiểm tra kết nối, chạy migration rồi mới phục vụ HTTP; shutdown đóng pool sau khi HTTP server đã drain.
- Dependency được ghim tương thích Go 1.21 thay vì chấp nhận nâng toolchain ngoài ý muốn.
- Thay bộ sinh code dùng `math/rand` không thread-safe bằng `crypto/rand`.
- Thêm PostgreSQL integration test có isolation schema, kiểm tra reconnect persistence, password, atomic visit, unique code và cascade deletion.

Trạng thái xác minh:

- Toàn bộ 38 unit/contract test chạy thành công.
- `go vet ./...` thành công.
- `go build` tạo binary thành công.
- PostgreSQL integration test tự động chạy khi có `TEST_DATABASE_URL`; môi trường hiện tại không cung cấp PostgreSQL/Docker nên test này được skip.
- Race detector vẫn cần CI Linux có CGO compiler.

### Giai đoạn 0 — Thiết lập baseline

**Mục tiêu:** biết chắc mã hiện tại build/test ra sao trước khi sửa.

Công việc:

1. Cài Go đúng version.
2. Chạy `gofmt`.
3. Chạy `go test ./...`.
4. Chạy `go test -race ./...`.
5. Chạy `go vet ./...`.
6. Ghi nhận coverage.
7. Tạo CI chạy các bước trên cho mỗi pull request.

Deliverables:

- CI xanh.
- Báo cáo baseline test/coverage.
- Không còn formatting drift.

### Giai đoạn 1 — Sửa correctness và security cấp cao

Công việc:

1. Sửa public URL builder.
2. Thêm atomic `SetIfAbsent`.
3. Trả snapshot từ storage.
4. Rehydrate user/role trong auth middleware.
5. Ownership cho info và stats.
6. Bảo vệ admin cuối cùng.
7. Bắt buộc admin password trong production.
8. Chuẩn hóa error mapping.

Deliverables:

- Bộ test regression tương ứng.
- Race detector xanh.
- Không còn stale-role authorization.

### Giai đoạn 2 — Persistence

Công việc:

1. Thiết kế schema PostgreSQL.
2. Tách repository interfaces.
3. Migration tooling.
4. Implement user repository.
5. Implement URL repository.
6. Unique constraints và transactional behavior.
7. Cleanup expired records.
8. Test restart và multi-instance.

Deliverables:

- Dữ liệu tồn tại qua restart.
- Có migration up/down hoặc chiến lược migration rõ.
- Có integration test với database.

### Giai đoạn 3 — Hardening production

Công việc:

1. Rate limiting.
2. Security headers/CORS allowlist.
3. Trusted proxies.
4. Structured logs và metrics.
5. Readiness/liveness.
6. Graceful shutdown cho mọi dependency.
7. Backup/restore procedure.
8. Secret rotation/token revocation strategy.

Deliverables:

- Deployment checklist có bằng chứng.
- Dashboard/log đủ để chẩn đoán lỗi.
- Threat model ngắn cho auth, redirects và admin.

### Giai đoạn 4 — API/frontend quality

Công việc:

1. API v1 rõ ràng.
2. Bỏ TTL khỏi redirect URL.
3. Pagination/sorting.
4. OpenAPI.
5. Tách frontend assets.
6. Session validation khi reload.
7. Browser end-to-end tests.
8. Accessibility và responsive review.

Deliverables:

- API contract ổn định.
- Frontend không dựa vào stale local user.
- E2E test cho login → create → redirect → stats → delete.

---

## 8. Kế hoạch test dự định bổ sung

### Unit test

- Token hợp lệ, sai signature, hết hạn, payload lỗi.
- URL không có hostname.
- URL scheme bị cấm.
- TTL mặc định.
- Collision random code.
- `SetIfAbsent`.
- Admin cuối cùng.
- Username/email normalization.

### Authorization test

- User A không đọc info URL của user B.
- User A không xóa URL của user B.
- User chỉ xem stats của mình.
- Admin xem toàn hệ thống.
- User bị xóa không dùng được token.
- Admin bị hạ role mất quyền ngay.
- Token role cũ không có hiệu lực.

### Concurrency test

- Nhiều request tạo cùng custom code.
- Redirect song song tăng visit chính xác.
- List URL đồng thời với redirect.
- Cleanup đồng thời với get/delete.
- `Storage.Stop` an toàn nếu lifecycle thay đổi.

### Integration test

- Khởi tạo full router giống production.
- Static files và SPA fallback.
- `/api` unknown route trả JSON 404.
- Reverse proxy headers.
- Graceful shutdown.
- PostgreSQL constraints và transaction.

### End-to-end test

1. Register.
2. Login.
3. Tạo short URL.
4. Copy/open short URL.
5. Kiểm tra visit tăng.
6. Xóa URL.
7. Admin đổi role.
8. Token cũ mất quyền.

---

## 9. Những thay đổi không nên làm vội

Một số ý tưởng hấp dẫn nhưng chưa nên ưu tiên trước correctness và persistence:

- Microservices.
- Kubernetes.
- Redis chỉ để “cho hiện đại”.
- Analytics phức tạp.
- QR code.
- Custom domain đa tenant.
- UI framework lớn.

Monolith hiện tại phù hợp với quy mô dự án. Nên giữ kiến trúc đơn giản, nhưng làm rõ interface và đảm bảo tính đúng đắn trước.

---

## 10. Thứ tự công việc cụ thể đề xuất

Nếu bắt đầu cải tiến ngay, thứ tự hợp lý là:

1. Thiết lập Go toolchain và CI baseline.
2. Viết regression test cho proxy URL, ownership và stale role.
3. Sửa public URL builder.
4. Sửa `SetIfAbsent` và snapshot copying.
5. Sửa middleware lấy role hiện hành.
6. Sửa ownership/stats.
7. Bảo vệ admin invariants và config secrets.
8. Chuẩn hóa domain errors.
9. Đưa persistence vào PostgreSQL.
10. Thêm rate limit, logs, metrics và security headers.
11. Refactor API/frontend.
12. Hợp nhất và cập nhật tài liệu.

---

## 11. Định nghĩa “production-ready” cho GoShorty

Chỉ nên gắn nhãn production-ready khi đạt tối thiểu:

- Dữ liệu không mất khi restart.
- Có migration và backup strategy.
- CI chạy build, test, race và vet.
- Không có race đã biết.
- Token/user role có thể revoke hoặc cập nhật tức thời.
- Ownership nhất quán trên mọi endpoint.
- Admin bootstrap an toàn.
- Rate limit cho auth.
- Public URL chính xác sau proxy.
- CORS/trusted proxy/security headers được cấu hình.
- Có structured logs và health/readiness.
- Có integration test và một happy-path E2E test.
- Tài liệu triển khai khớp mã thực tế.

---

## 12. Kết luận

GoShorty có nền tảng tốt cho một dự án Go nhỏ:

- Luồng dễ theo dõi.
- Package boundaries tương đối rõ.
- Có authentication, authorization và TTL.
- Đã sửa nhiều lỗi bảo mật cơ bản.
- Có lượng tài liệu và test tốt hơn nhiều dự án demo cùng quy mô.

Điểm cần thay đổi lớn nhất không phải là thêm tính năng, mà là nâng độ tin cậy:

- Từ in-memory sang persistent.
- Từ token snapshot sang authorization theo trạng thái hiện hành.
- Từ “có mutex” sang concurrency semantics thực sự atomic.
- Từ cấu hình theo request sang public origin đáng tin cậy.
- Từ test chức năng cơ bản sang regression, race, integration và E2E.

Khi hoàn thành P0 và P1, dự án sẽ chuyển từ demo tốt sang backend đáng tin cậy. Khi hoàn thành persistence và production hardening, GoShorty mới có cơ sở kỹ thuật để phục vụ người dùng thật ổn định.
