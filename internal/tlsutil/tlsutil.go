// Package tlsutil 提供骨架期/测试用的 TLS 证书与配置工具。
//
// 单向 TLS（agent 验 server）：agent 用内置 CA 验证 server 身份防中间人，
// server 不验 agent（mTLS 在注册后凭叶子证书，是下一步）。
// 证书全链路 ECC P-256，对齐《密钥签发与证书管理》§1.1。
//
// 注意：GenerateDevCert 仅用于开发/测试自签；正式根 CA 经 license 注入（ADR-0011）。
package tlsutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"time"
)

// GenerateDevCert 生成自签证书链：自签 CA → server 叶子证书（由 CA 签）。
//
// 返回 PEM：caPEM（CA 证书，给 agent 当信任锚）、certPEM（server 叶子证书）、
// keyPEM（server 私钥）。hosts 填进叶子证书 SAN（区分 IP / DNS）。
func GenerateDevCert(hosts []string) (caPEM, certPEM, keyPEM []byte, err error) {
	// 1 生成 CA 私钥 + 自签 CA 证书（IsCA=true）。
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("生成 CA 私钥: %w", err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Topus Dev Root CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("自签 CA 证书: %w", err)
	}

	// 2 生成 server 私钥 + 叶子证书模板（SAN 区分 IP/DNS），用 CA 私钥签发。
	srvKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("生成 server 私钥: %w", err)
	}
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "topus-server"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(1, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			leafTmpl.IPAddresses = append(leafTmpl.IPAddresses, ip)
		} else {
			leafTmpl.DNSNames = append(leafTmpl.DNSNames, h)
		}
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("解析 CA 证书: %w", err)
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, &srvKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("签发叶子证书: %w", err)
	}

	// 3 编码为 PEM。
	caPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})
	srvKeyDER, err := x509.MarshalECPrivateKey(srvKey)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("编码 server 私钥: %w", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: srvKeyDER})
	return caPEM, certPEM, keyPEM, nil
}

// ServerTLSConfig 用 server 证书 + 私钥构建 server 端 TLS 配置（单向，不验客户端）。
func ServerTLSConfig(certPEM, keyPEM []byte) (*tls.Config, error) {
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("加载 server 证书/私钥: %w", err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
	}, nil
}

// ClientTLSConfig 用 CA 构建 agent 端 TLS 配置（单向：用 CA 验 server）。
func ClientTLSConfig(caPEM []byte) (*tls.Config, error) {
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("CA PEM 无有效证书")
	}
	return &tls.Config{
		RootCAs:    pool,
		MinVersion: tls.VersionTLS13,
	}, nil
}
