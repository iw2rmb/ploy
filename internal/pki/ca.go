// Package pki provides certificate authority and certificate management utilities
// for the simplified Ploy server/node mTLS architecture.
package pki

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"time"
)

const (
	// CAValidity is the default validity period for the cluster CA certificate.
	CAValidity = 10 * 365 * 24 * time.Hour // 10 years

	// NodeCertValidity is the default validity period for node certificates.
	NodeCertValidity = 365 * 24 * time.Hour // 1 year

	// ServerCertValidity is the default validity period for server certificates.
	ServerCertValidity = 365 * 24 * time.Hour // 1 year

	certSerialBitSize = 128
)

// CABundle represents a certificate authority bundle with both certificate and private key.
type CABundle struct {
	CertPEM string
	KeyPEM  string
	Cert    *x509.Certificate
	Key     *ecdsa.PrivateKey
}

// IssuedCert represents an issued certificate with metadata.
type IssuedCert struct {
	CertPEM     string
	KeyPEM      string
	Serial      string
	Fingerprint string
	NotBefore   time.Time
	NotAfter    time.Time
	Cert        *x509.Certificate
	Key         *ecdsa.PrivateKey
}

// GenerateCA creates a new certificate authority for the cluster.
// The clusterID is used in the CA subject common name.
func GenerateCA(clusterID string, now time.Time) (*CABundle, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate CA key: %w", err)
	}

	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   fmt.Sprintf("ploy-cluster-%s-ca", clusterID),
			Organization: []string{"Ploy"},
		},
		NotBefore:             now.Add(-1 * time.Minute),
		NotAfter:              now.Add(CAValidity),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		return nil, fmt.Errorf("create CA certificate: %w", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("parse CA certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("marshal CA private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return &CABundle{
		CertPEM: string(certPEM),
		KeyPEM:  string(keyPEM),
		Cert:    cert,
		Key:     priv,
	}, nil
}

// IssueServerCert issues a server certificate signed by the CA.
// It sets the subject CN to "ployd-<clusterID>" and includes a DNS SAN
// of the form "ployd.<clusterID>.ploy". The provided serverIP is also
// added to IP SANs for direct addressing.
func IssueServerCert(ca *CABundle, clusterID, serverIP string, now time.Time) (*IssuedCert, error) {
	return issueCert(ca, fmt.Sprintf("ployd-%s", clusterID), []string{fmt.Sprintf("ployd.%s.ploy", clusterID)}, []string{serverIP}, now, ServerCertValidity)
}

// SignNodeCSR signs a node certificate signing request using the cluster CA.
// Returns the signed certificate with metadata for persistence.
func SignNodeCSR(ca *CABundle, csrPEM []byte, now time.Time) (*IssuedCert, error) {
	block, _ := pem.Decode(csrPEM)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return nil, errors.New("invalid CSR PEM: missing or wrong block type")
	}

	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse CSR: %w", err)
	}

	if err := csr.CheckSignature(); err != nil {
		return nil, fmt.Errorf("verify CSR signature: %w", err)
	}

	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}

	template := &x509.Certificate{
		SerialNumber:   serial,
		Subject:        csr.Subject,
		NotBefore:      now.Add(-1 * time.Minute),
		NotAfter:       now.Add(NodeCertValidity),
		KeyUsage:       x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:    []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		DNSNames:       csr.DNSNames,
		IPAddresses:    csr.IPAddresses,
		EmailAddresses: csr.EmailAddresses,
		URIs:           csr.URIs,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, ca.Cert, csr.PublicKey, ca.Key)
	if err != nil {
		return nil, fmt.Errorf("sign certificate: %w", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("parse signed certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	fingerprint := sha256.Sum256(certDER)

	return &IssuedCert{
		CertPEM:     string(certPEM),
		Serial:      hex.EncodeToString(serial.Bytes()),
		Fingerprint: hex.EncodeToString(fingerprint[:]),
		NotBefore:   cert.NotBefore,
		NotAfter:    cert.NotAfter,
		Cert:        cert,
	}, nil
}

// GenerateNodeCSR generates a private key and CSR for a node.
// The nodeID is used in the certificate CN as "node:<nodeID>".
// The nodeIP is included in SANs along with the DNS name.
func GenerateNodeCSR(nodeID, clusterID, nodeIP string) (*IssuedCert, []byte, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate node key: %w", err)
	}

	cn := fmt.Sprintf("node:%s", nodeID)
	dnsName := fmt.Sprintf("node-%s.%s.ploy", nodeID, clusterID)

	var ipAddrs []net.IP
	if nodeIP != "" {
		parsed := net.ParseIP(nodeIP)
		if parsed != nil {
			ipAddrs = append(ipAddrs, parsed)
		}
	}

	template := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   cn,
			Organization: []string{"Ploy"},
		},
		DNSNames:    []string{dnsName},
		IPAddresses: ipAddrs,
	}

	csrDER, err := x509.CreateCertificateRequest(rand.Reader, template, priv)
	if err != nil {
		return nil, nil, fmt.Errorf("create CSR: %w", err)
	}

	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal node private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	keyBundle := &IssuedCert{
		KeyPEM: string(keyPEM),
		Key:    priv,
	}

	return keyBundle, csrPEM, nil
}

// LoadCA loads a CA bundle from PEM-encoded certificate and private key.
func LoadCA(certPEM, keyPEM string) (*CABundle, error) {
	certBlock, _ := pem.Decode([]byte(certPEM))
	if certBlock == nil || certBlock.Type != "CERTIFICATE" {
		return nil, errors.New("invalid CA certificate PEM")
	}

	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse CA certificate: %w", err)
	}

	keyBlock, _ := pem.Decode([]byte(keyPEM))
	if keyBlock == nil {
		return nil, errors.New("invalid CA key PEM")
	}

	var key *ecdsa.PrivateKey
	switch keyBlock.Type {
	case "EC PRIVATE KEY":
		key, err = x509.ParseECPrivateKey(keyBlock.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse EC private key: %w", err)
		}
	case "PRIVATE KEY":
		keyAny, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse PKCS8 private key: %w", err)
		}
		var ok bool
		key, ok = keyAny.(*ecdsa.PrivateKey)
		if !ok {
			return nil, errors.New("CA private key must be ECDSA")
		}
	default:
		return nil, fmt.Errorf("unsupported key type: %s", keyBlock.Type)
	}

	return &CABundle{
		CertPEM: certPEM,
		KeyPEM:  keyPEM,
		Cert:    cert,
		Key:     key,
	}, nil
}

// issueCert is a helper to issue a certificate with the given parameters.
func issueCert(ca *CABundle, cn string, dnsNames []string, ips []string, now time.Time, validity time.Duration) (*IssuedCert, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}

	var ipAddrs []net.IP
	for _, ip := range ips {
		parsed := net.ParseIP(ip)
		if parsed != nil {
			ipAddrs = append(ipAddrs, parsed)
		}
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   cn,
			Organization: []string{"Ploy"},
		},
		NotBefore:   now.Add(-1 * time.Minute),
		NotAfter:    now.Add(validity),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		DNSNames:    dnsNames,
		IPAddresses: ipAddrs,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, ca.Cert, &priv.PublicKey, ca.Key)
	if err != nil {
		return nil, fmt.Errorf("create certificate: %w", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("marshal private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	fingerprint := sha256.Sum256(certDER)

	return &IssuedCert{
		CertPEM:     string(certPEM),
		KeyPEM:      string(keyPEM),
		Serial:      hex.EncodeToString(serial.Bytes()),
		Fingerprint: hex.EncodeToString(fingerprint[:]),
		NotBefore:   cert.NotBefore,
		NotAfter:    cert.NotAfter,
		Cert:        cert,
		Key:         priv,
	}, nil
}

// randomSerial generates a random serial number for certificates.
func randomSerial() (*big.Int, error) {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), certSerialBitSize)
	serial, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, fmt.Errorf("generate serial number: %w", err)
	}
	if serial.Sign() == 0 {
		return randomSerial()
	}
	return serial, nil
}
