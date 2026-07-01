//go:build !embedosq

package osq

// extractEmbedded：普通构建**不内嵌** osqueryd，直接返回 false（走 fetch / 同目录查找）。
// 只有 `-tags embedosq` 的发布构建才用 embed_osqueryd.go 的实现内嵌并解压。
func extractEmbedded() (string, bool, error) {
	return "", false, nil
}
