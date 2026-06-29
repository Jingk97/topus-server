// Command gen-certs 生成开发用自签证书到目录（ca.pem / server.pem / server-key.pem）。
// 仅供本地开发与冒烟使用——正式根 CA 经 license 注入（ADR-0011），勿用于生产。
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/Jingk97/topus-server/internal/tlsutil"
)

func main() {
	dir := flag.String("dir", "certs", "输出目录")
	hosts := flag.String("hosts", "127.0.0.1,localhost", "证书 SAN，逗号分隔")
	flag.Parse()

	// 1 生成自签 CA + server 叶子证书（ECC P-256）。
	caPEM, certPEM, keyPEM, err := tlsutil.GenerateDevCert(strings.Split(*hosts, ","))
	if err != nil {
		log.Fatalf("生成证书失败: %v", err)
	}

	// 2 写文件；私钥权限收紧到 0600。
	if err := os.MkdirAll(*dir, 0o755); err != nil {
		log.Fatalf("建目录失败: %v", err)
	}
	writes := []struct {
		name string
		data []byte
		perm os.FileMode
	}{
		{"ca.pem", caPEM, 0o644},
		{"server.pem", certPEM, 0o644},
		{"server-key.pem", keyPEM, 0o600},
	}
	for _, w := range writes {
		p := filepath.Join(*dir, w.name)
		if err := os.WriteFile(p, w.data, w.perm); err != nil {
			log.Fatalf("写 %s 失败: %v", p, err)
		}
	}
	log.Printf("已生成开发证书到 %s/（ca.pem, server.pem, server-key.pem）", *dir)
}
