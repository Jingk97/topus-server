.PHONY: gen tidy build test smoke clean

# proto 生成：buf → go(消息) + go-grpc(服务桩)，输出回 api/ 源码同目录
gen:
	buf generate

tidy:
	go mod tidy

# 一应用一二进制，统一输出到根 bin/（全局规则第7节，bin/ 已 gitignore）
build:
	@mkdir -p bin
	go build -o bin/topus-server ./cmd/server
	go build -o bin/topus-agent  ./cmd/agent

# 单元/契约测试：bufconn 内存管道，不起真实进程
test:
	go test ./...

# 端到端冒烟：起真 server 进程 → agent test 子命令真连 → 验证 ok
smoke: build
	@./scripts/smoke.sh

clean:
	@rm -rf bin
