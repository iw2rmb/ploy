package deploy

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"strings"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// leafProfile describes SANs for leaf certificates.
//
//nolint:unused // used by planned CA rotation flow
type leafProfile struct {
	DNSNames    []string
	IPAddresses []net.IP
}

//nolint:unused // helper reserved for CA rotation rollout
func decodeCABundleMaterials(bundle CABundle) (*ecdsa.PrivateKey, *x509.Certificate, error) {
	keyBlock, _ := pem.Decode([]byte(bundle.KeyPEM))
	if keyBlock == nil {
		return nil, nil, errors.New("deploy: decode CA private key: missing PEM block")
	}
	keyAny, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("deploy: parse CA private key: %w", err)
	}
	privateKey, ok := keyAny.(*ecdsa.PrivateKey)
	if !ok {
		return nil, nil, errors.New("deploy: CA private key must be ecdsa")
	}
	certBlock, _ := pem.Decode([]byte(bundle.CertificatePEM))
	if certBlock == nil {
		return nil, nil, errors.New("deploy: decode CA certificate: missing PEM block")
	}
	caCert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("deploy: parse CA certificate: %w", err)
	}
	return privateKey, caCert, nil
}

//nolint:unused // retained for future CA rotation tooling
func generateCABundle(clusterID string, now time.Time, validity time.Duration) (CABundle, *ecdsa.PrivateKey, *x509.Certificate, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return CABundle{}, nil, nil, fmt.Errorf("deploy: generate CA key: %w", err)
	}
	serial, err := randomSerial()
	if err != nil {
		return CABundle{}, nil, nil, err
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:         fmt.Sprintf("ploy-%s-root", clusterID),
			Organization:       []string{"Ploy Deployment"},
			OrganizationalUnit: []string{"Control Plane"},
		},
		NotBefore:             now.Add(-1 * time.Minute),
		NotAfter:              now.Add(validity),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
		MaxPathLenZero:        false,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		return CABundle{}, nil, nil, fmt.Errorf("deploy: create CA certificate: %w", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return CABundle{}, nil, nil, fmt.Errorf("deploy: marshal CA private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	version := buildVersion(now)
	return CABundle{
		Version:        version,
		SerialNumber:   serial.Text(16),
		Subject:        template.Subject.String(),
		IssuedAt:       now,
		ExpiresAt:      template.NotAfter,
		CertificatePEM: string(certPEM),
		KeyPEM:         string(keyPEM),
	}, priv, template, nil
}

//nolint:unused // reserved for CA rotation issuance helpers
func issueLeafCertificate(nodeID, role string, ca CABundle, caCert *x509.Certificate, caKey *ecdsa.PrivateKey, now time.Time, validity time.Duration, previousVersion string, profile *leafProfile) (LeafCertificate, error) {
	role = strings.TrimSpace(role)
	if role == "" {
		return LeafCertificate{}, errors.New("deploy: certificate role required")
	}
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return LeafCertificate{}, fmt.Errorf("deploy: generate %s key for %s: %w", role, nodeID, err)
	}
	serial, err := randomSerial()
	if err != nil {
		return LeafCertificate{}, err
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:         fmt.Sprintf("%s-%s", role, nodeID),
			Organization:       []string{"Ploy Deployment"},
			OrganizationalUnit: []string{"Ploy " + role},
		},
		NotBefore: now.Add(-1 * time.Minute),
		NotAfter:  now.Add(validity),
		KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
			x509.ExtKeyUsageServerAuth,
		},
	}
	if profile != nil {
		template.DNSNames = append(template.DNSNames, profile.DNSNames...)
		template.IPAddresses = append(template.IPAddresses, profile.IPAddresses...)
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, caCert, &priv.PublicKey, caKey)
	if err != nil {
		return LeafCertificate{}, fmt.Errorf("deploy: create %s certificate for %s: %w", role, nodeID, err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return LeafCertificate{}, fmt.Errorf("deploy: marshal %s private key for %s: %w", role, nodeID, err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	return LeafCertificate{
		NodeID:          domaintypes.NodeID(nodeID), // Convert to domain type
		Usage:           role,
		Version:         buildVersion(now),
		ParentVersion:   ca.Version,
		SerialNumber:    hex.EncodeToString(serial.Bytes()),
		CertificatePEM:  string(certPEM),
		KeyPEM:          string(keyPEM),
		IssuedAt:        now,
		ExpiresAt:       now.Add(validity),
		PreviousVersion: previousVersion,
	}, nil
}

//nolint:unused // used by generateCABundle/issueLeafCertificate during rotation work
func randomSerial() (*big.Int, error) {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), certSerialBitSize)
	serial, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, fmt.Errorf("deploy: generate serial number: %w", err)
	}
	if serial.Sign() == 0 {
		return randomSerial()
	}
	return serial, nil
}

//nolint:unused // kept for versioned CA/leaf metadata helpers
func buildVersion(now time.Time) string {
	ts := now.UTC().Format("20060102T150405Z")
	randomBytes := make([]byte, 4)
	if _, err := rand.Read(randomBytes); err != nil {
		return ts
	}
	return fmt.Sprintf("%s-%s", ts, hex.EncodeToString(randomBytes))
}
