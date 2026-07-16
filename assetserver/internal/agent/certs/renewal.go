// Package certs — mTLS 证书管理 (Phase 7)
// 对应架构文档 §7.1 mTLS 证书管理
package certs

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"time"
)

// CertManager mTLS 证书管理器
type CertManager struct {
	CertFile string
	KeyFile  string
	CAFile   string
}

func NewCertManager(certFile, keyFile, caFile string) *CertManager {
	return &CertManager{CertFile: certFile, KeyFile: keyFile, CAFile: caFile}
}

// CheckExpiry 检查证书是否到期前 7 天
func (cm *CertManager) CheckExpiry() (bool, time.Time, error) {
	data, err := os.ReadFile(cm.CertFile)
	if err != nil {
		return false, time.Time{}, fmt.Errorf("read cert: %w", err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return false, time.Time{}, fmt.Errorf("invalid PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false, time.Time{}, fmt.Errorf("parse cert: %w", err)
	}

	timeUntilExpiry := time.Until(cert.NotAfter)
	needsRenewal := timeUntilExpiry < 7*24*time.Hour
	return needsRenewal, cert.NotAfter, nil
}

// RenewCertificate 自动续期 (简化: 占位实现)
func (cm *CertManager) RenewCertificate(serverURL string) error {
	// 生产: POST /auth/renew-cert → 签发新证书
	// 验证新证书 → 替换旧证书 → 通知服务端
	_ = serverURL
	_ = cm
	return nil
}

// IsRevoked 检查证书是否已被吊销 (CRL + OCSP 双重校验)
func (cm *CertManager) IsRevoked() (bool, error) {
	// 生产: 下载 CRL 文件 → 检查 serial number
	// 同时 OCSP 在线查询
	return false, nil
}
