package certificate

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"

	"k8s.io/client-go/util/certificate"
)

type MemoryStore struct {
	Certificate *tls.Certificate
}

func (m *MemoryStore) Current() (*tls.Certificate, error) {
	if m.Certificate == nil {
		noKeyErr := certificate.NoCertKeyError("derp")

		return nil, &noKeyErr
	}

	return m.Certificate, nil
}

func (m *MemoryStore) Update(certData, keyData []byte) (*tls.Certificate, error) {
	cert, err := tls.X509KeyPair(certData, keyData)
	if err != nil {
		return nil, err
	}

	certs, err := x509.ParseCertificates(cert.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("unable to parse certificate data: %w", err)
	}

	cert.Leaf = certs[0]
	m.Certificate = &cert

	return &cert, err
}
