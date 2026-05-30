# Script to generate Go code from protobuf using Docker
# No local protoc installation required!

Write-Host "🔧 Running Docker to compile bidding.proto..." -ForegroundColor Cyan

# 1. Ensure target directory exists
New-Item -ItemType Directory -Force -Path "./proto" | Out-Null

# 2. Run protoc using Docker protobuf container
docker run --rm `
  -v "${PWD}/../..:/workspace" `
  -w /workspace `
  rvolykh/protobuf-tool:latest `
  protoc `
    --go_out=./nft-auction/services/bidding-service/proto `
    --go-grpc_out=./nft-auction/services/bidding-service/proto `
    --go_opt=paths=source_relative `
    --go-grpc_opt=paths=source_relative `
    -I ./proto `
    bidding.proto

if ($LASTEXITCODE -eq 0) {
    Write-Host "✅ Protobuf compile SUCCESS!" -ForegroundColor Green
    Write-Host "👉 Code generated inside: ./proto/" -ForegroundColor Green
} else {
    Write-Host "❌ ERROR: Protobuf compile failed! Check if Docker Desktop is running." -ForegroundColor Red
}
