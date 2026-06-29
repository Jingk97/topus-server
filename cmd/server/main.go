// Command topus-server 是 server 端可执行入口。
// 起 gRPC server 并注册 health.Ping。提供 --tls-cert/--tls-key 则启用单向 TLS
// （server 出示证书，agent 用 CA 验），否则明文（骨架期）。
package main

import (
	"crypto/tls"
	"flag"
	"log"
	"os"

	"github.com/Jingk97/topus-server/internal/server"
	"github.com/Jingk97/topus-server/internal/tlsutil"
)

func main() {
	addr := flag.String("addr", ":9090", "gRPC 监听地址")
	tlsCert := flag.String("tls-cert", "", "server 证书 PEM 路径（空=明文）")
	tlsKey := flag.String("tls-key", "", "server 私钥 PEM 路径")
	flag.Parse()

	// 1 两个 TLS 参数都给齐才启用 TLS；否则明文（骨架期）。
	var tlsConf *tls.Config
	mode := "明文，骨架期"
	if *tlsCert != "" && *tlsKey != "" {
		certPEM, err := os.ReadFile(*tlsCert)
		if err != nil {
			log.Fatalf("读 tls-cert 失败: %v", err)
		}
		keyPEM, err := os.ReadFile(*tlsKey)
		if err != nil {
			log.Fatalf("读 tls-key 失败: %v", err)
		}
		tlsConf, err = tlsutil.ServerTLSConfig(certPEM, keyPEM)
		if err != nil {
			log.Fatalf("构建 server TLS 配置失败: %v", err)
		}
		mode = "单向 TLS"
	}

	// 2 装配并起 server（注册在 Serve 之前，见 internal/server）。
	s, lis, err := server.New(*addr, tlsConf)
	if err != nil {
		log.Fatalf("起 server 失败: %v", err)
	}
	log.Printf("topus-server 启动，gRPC 监听 %s（%s）", *addr, mode)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
