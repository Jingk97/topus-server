.PHONY: gen tidy build test embed-check smoke certs osquery collect agent-embed clean

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

# 单元/契约测试：bufconn 内存管道，不起真实进程。先过 embed 编译门。
test: embed-check
	go test ./...

# embed 编译门：assets/osqueryd.gz 不入 git（靠 build 脚本 fetch 生成），默认构建编译不到
# embed_osqueryd.go 的解压/权限/TOCTOU 代码。用一字节占位 gz 让 `-tags embedosq` 能编译，
# 守住 embed 分支的编译信号（笔误/签名漂移会被抓到）；已有真实 gz 时直接编译不动它。
embed-check:
	@GZ=internal/agent/osq/assets/osqueryd.gz; \
	if [ -f $$GZ ]; then \
	  go build -tags embedosq -o /dev/null ./cmd/agent && echo "embed build OK (existing asset)"; \
	else \
	  printf '' | gzip > $$GZ; \
	  go build -tags embedosq -o /dev/null ./cmd/agent; ret=$$?; \
	  rm -f $$GZ; \
	  if [ $$ret -eq 0 ]; then echo "embed build OK (placeholder)"; else exit $$ret; fi; \
	fi

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

# 构建内嵌 osqueryd 的单文件 linux agent（产品化形态）：make agent-embed [ARCH=amd64|arm64]
agent-embed:
	bash deploy/build-agent-embed.sh $(ARCH)

clean:
	@rm -rf bin
