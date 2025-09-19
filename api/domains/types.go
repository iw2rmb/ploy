package domains

// DomainRequest represents a domain registration request.
type DomainRequest struct {
	Domain       string `json:"domain"`
	Certificate  string `json:"certificate,omitempty"`
	CertProvider string `json:"cert_provider,omitempty"`
}

// DomainResponse describes responses returned by domain endpoints.
type DomainResponse struct {
	Status       string             `json:"status"`
	App          string             `json:"app,omitempty"`
	Domain       string             `json:"domain,omitempty"`
	Domains      []string           `json:"domains,omitempty"`
	Message      string             `json:"message,omitempty"`
	Certificate  *CertificateInfo   `json:"certificate,omitempty"`
	Certificates []*CertificateInfo `json:"certificates,omitempty"`
}

// CertificateInfo reports certificate details in responses.
type CertificateInfo struct {
	Domain    string `json:"domain"`
	Status    string `json:"status"`
	Provider  string `json:"provider"`
	IssuedAt  string `json:"issued_at,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
	AutoRenew bool   `json:"auto_renew"`
}
