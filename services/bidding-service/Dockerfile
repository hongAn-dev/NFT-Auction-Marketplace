# =================================================================
# STAGE 1: Builder (Môi trường build code cực kỳ đầy đủ)
# =================================================================
FROM golang:1.23-alpine AS builder

# Thiết lập thư mục làm việc bên trong container
WORKDIR /app

# Sao chép các file định nghĩa dependency (Go Modules) vào trước
# Việc sao chép riêng lẻ này giúp Docker cache lại layer tải dependencies (go mod download)
# Chỉ khi go.mod thay đổi thì Docker mới tải lại, giúp tiết kiệm thời gian build rất nhiều!
COPY go.mod ./

# Chạy lệnh tải các thư viện phụ thuộc (Hiện tại chưa có dependency ngoài nên lệnh này chạy rất nhanh)
RUN go mod download

# Sao chép toàn bộ mã nguồn vào trong container
COPY . .

# Biên dịch ứng dụng Go thành một file thực thi (binary) tĩnh độc lập
# - CGO_ENABLED=0: Tắt CGO để đảm bảo file thực thi không phụ thuộc vào thư viện liên kết động C trên OS
# - GOOS=linux: Đảm bảo biên dịch ra file chạy được trên môi trường Linux bên trong Docker
# - -o server ./cmd/server/main.go: Đặt tên file đầu ra là "server" từ file nguồn main.go
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server/main.go

# =================================================================
# STAGE 2: Runtime (Môi trường chạy siêu gọn nhẹ, tối ưu bảo mật)
# =================================================================
FROM alpine:3.19

# Cài đặt thêm các chứng chỉ bảo mật HTTPS (CA Certificates) và múi giờ (Timezone data)
RUN apk --no-cache add ca-certificates tzdata

# Thiết lập thư mục chạy ứng dụng
WORKDIR /root/

# Chỉ copy ĐÚNG file thực thi "server" đã biên dịch từ Stage 1 sang Stage 2
# Điều này giúp loại bỏ hoàn toàn bộ cài đặt Go Compiler, các mã nguồn thừa, giúp dung lượng image cực kỳ nhẹ (< 20MB)
COPY --from=builder /app/server .

# Khai báo port lắng nghe (Bidding service chạy gRPC trên port 50051 theo ARCHITECTURE.md)
EXPOSE 50051

# Lệnh khởi chạy ứng dụng khi container khởi động
CMD ["./server"]
