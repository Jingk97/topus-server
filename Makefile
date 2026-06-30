.PHONY: gen tidy build test smoke certs osquery collect clean

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

# 生成开发用自签证书到 certs/（ca.pem / server.pem / server-key.pem）
certs:
	go run ./cmd/gen-certs -dir=certs -hosts=127.0.0.1,localhost

# 端到端冒烟：起真 server(单向 TLS) → agent test --ca 真连 → 验证 ok
smoke: build
	@./scripts/smoke.sh

# 拉取项目内 osqueryd（不装宿主机；mac 为完整 .app bundle，见 deploy/osquery/fetch.sh）
osquery:
	bash deploy/osquery/fetch.sh

# 本地采集演示：起 osqueryd 采 host+进程，结构化日志 + 输出快照 JSON
collect: build osquery
	./bin/topus-agent collect

clean:
	@rm -rf bin
