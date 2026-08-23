# BENZHI_README

## 项目说明

- 项目：11DingKing/yellow-sea-forest-operations-20260823
- 项目用途：A Go backend for operating the Yellow Sea coastal forest. It tracks a stewardship case from ranger or community intake through ecological survey, restoration planning, field work, inspection, visitor opening, and a follow-up window. The workflow reflects the forest's three-generation hand-off: protect the forest first, then make visitor, education, lodging, understory-economy, and carbon-finance services dependable.
- Go 工具链：`golang:1.23.0`
- 前端工具链：无

## 标准构建、运行和测试命令

进入容器后执行：

```bash
# 编译
cd '/app' && GOTOOLCHAIN=local go build ./...

# 启动
cd '/app' && GOTOOLCHAIN=local go run ./cmd/server

# 测试
cd '/app' && GOTOOLCHAIN=local go test ./...
```

## Docker 构建和进入容器

```bash
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh benzhi-task-91-amd64 linux/amd64
./build_benzhi_docker.sh benzhi-task-91-arm64 linux/arm64
docker run -it benzhi-task-91-amd64:latest
docker run -it --platform linux/arm64 benzhi-task-91-arm64:latest
```

## 题目验证命令

1. 预期退出码 0：`go test ./internal/service -run '^TestForestParcelCannotOpenWithOpenPatrolIssue$' -count=1`
2. 预期退出码 0：`go test ./...`
3. 预期退出码 0：`GOTOOLCHAIN=local go build -buildvcs=false ./... && GOTOOLCHAIN=local go vet ./...`
