package httpserver

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/controlplane/registry"
	cpsecurity "github.com/iw2rmb/ploy/internal/controlplane/security"
	"github.com/iw2rmb/ploy/internal/deploy"
	"github.com/iw2rmb/ploy/internal/version"
	dto "github.com/prometheus/client_model/go"
)

func (s *controlPlaneServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	resp := map[string]any{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *controlPlaneServer) handleStatusSummary(w http.ResponseWriter, r *http.Request) {
	if !s.ensureEtcd(w) {
		return
	}
	status := http.StatusOK

	clusterID := strings.TrimSpace(r.URL.Query().Get("cluster_id"))
	if clusterID == "" {
		status = http.StatusBadRequest
		writeErrorMessage(w, status, "cluster_id query parameter required")
		return
	}

	queueDepths := s.collectQueueDepths()
	totalDepth := 0.0
	priorities := make([]map[string]any, 0, len(queueDepths))
	for priority, depth := range queueDepths {
		priorities = append(priorities, map[string]any{
			"priority": priority,
			"depth":    depth,
		})
		totalDepth += depth
	}
	sort.Slice(priorities, func(i, j int) bool {
		pi := fmt.Sprintf("%v", priorities[i]["priority"])
		pj := fmt.Sprintf("%v", priorities[j]["priority"])
		return pi < pj
	})

	workerStats := map[string]any{
		"total": 0,
		"phases": map[string]int{
			"ready":       0,
			"registering": 0,
			"error":       0,
			"unknown":     0,
		},
	}

	reg, err := registry.NewWorkerRegistry(s.etcd, clusterID)
	if err != nil {
		status = http.StatusInternalServerError
		writeError(w, status, err)
		return
	}
	descriptors, err := reg.List(r.Context())
	if err != nil {
		status = http.StatusInternalServerError
		writeError(w, status, err)
		return
	}

	phases := workerStats["phases"].(map[string]int)
	for _, descriptor := range descriptors {
		phase := strings.TrimSpace(descriptor.Status.Phase)
		if phase == "" {
			phase = "unknown"
		}
		phases[phase]++
	}
	workerStats["total"] = len(descriptors)

	payload := map[string]any{
		"cluster_id": clusterID,
		"timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
		"queue": map[string]any{
			"total_depth": totalDepth,
			"priorities":  priorities,
		},
		"workers": workerStats,
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, status, payload)
}

func (s *controlPlaneServer) handleSecurityCA(w http.ResponseWriter, r *http.Request) {
	if !s.ensureEtcd(w) {
		return
	}
	clusterID := strings.TrimSpace(r.URL.Query().Get("cluster_id"))
	if clusterID == "" {
		writeErrorMessage(w, http.StatusBadRequest, "cluster_id query parameter required")
		return
	}
	manager, err := deploy.NewCARotationManager(s.etcd, clusterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	state, err := manager.State(r.Context())
	if err != nil {
		switch {
		case errors.Is(err, deploy.ErrPKINotBootstrapped):
			writeErrorMessage(w, http.StatusNotFound, "cluster PKI not bootstrapped")
		default:
			writeError(w, http.StatusInternalServerError, err)
		}
		return
	}
	trustHash := ""
	if store, err := cpsecurity.NewTrustStore(s.etcd, clusterID); err == nil {
		if bundle, _, err := store.Current(r.Context()); err == nil {
			trustHash = bundle.CABundleHash
		}
	}
	current := map[string]any{
		"version":       state.CurrentCA.Version,
		"serial_number": state.CurrentCA.SerialNumber,
	}
	if !state.CurrentCA.IssuedAt.IsZero() {
		current["issued_at"] = state.CurrentCA.IssuedAt.UTC().Format(time.RFC3339Nano)
	}
	if !state.CurrentCA.ExpiresAt.IsZero() {
		current["expires_at"] = state.CurrentCA.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	response := map[string]any{
		"cluster_id": clusterID,
		"current_ca": current,
		"workers": map[string]any{
			"total": len(state.Nodes.Workers),
		},
	}
	if len(state.Nodes.Beacons) > 0 {
		response["control_plane"] = map[string]any{
			"total": len(state.Nodes.Beacons),
		}
	}
	if trustHash != "" {
		response["trust_bundle_hash"] = trustHash
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *controlPlaneServer) handleControlPlaneCertificate(w http.ResponseWriter, r *http.Request) {
	if !s.ensureEtcd(w) {
		return
	}
	var req controlPlaneCertRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	clusterID := strings.TrimSpace(req.ClusterID)
	if clusterID == "" {
		writeErrorMessage(w, http.StatusBadRequest, "cluster_id is required")
		return
	}
	nodeID := strings.TrimSpace(req.NodeID)
	if nodeID == "" {
		nodeID = "control"
	}
	address := strings.TrimSpace(req.Address)
	if address == "" {
		writeErrorMessage(w, http.StatusBadRequest, "address is required")
		return
	}
	created, err := deploy.EnsureClusterPKI(r.Context(), s.etcd, clusterID, deploy.EnsurePKIOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if created {
		log.Printf("auto-bootstrapped cluster PKI for %s", clusterID)
	}
	manager, err := deploy.NewCARotationManager(s.etcd, clusterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	cert, err := manager.IssueControlPlaneCertificate(r.Context(), nodeID, address, time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	state, err := manager.State(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	resp := map[string]any{
		"cluster_id":  clusterID,
		"certificate": cert,
		"ca_bundle":   state.CurrentCA.CertificatePEM,
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *controlPlaneServer) handleVersion(w http.ResponseWriter, r *http.Request) {
	payload := map[string]any{
		"version":  strings.TrimSpace(version.Version),
		"commit":   strings.TrimSpace(version.Commit),
		"built_at": strings.TrimSpace(version.BuiltAt),
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	writeJSON(w, http.StatusOK, payload)
}

type controlPlaneCertRequest struct {
	ClusterID string `json:"cluster_id"`
	NodeID    string `json:"node_id"`
	Address   string `json:"address"`
}

func (s *controlPlaneServer) collectQueueDepths() map[string]float64 {
	depths := make(map[string]float64)
	if s.gatherer == nil {
		return depths
	}
	families, err := s.gatherer.Gather()
	if err != nil {
		return depths
	}
	for _, fam := range families {
		if fam == nil || fam.GetName() != "ploy_controlplane_queue_depth" {
			continue
		}
		for _, metric := range fam.GetMetric() {
			if metric == nil || metric.GetGauge() == nil {
				continue
			}
			priority := labelValue(metric, "priority")
			if priority == "" {
				priority = "default"
			}
			depths[priority] = metric.GetGauge().GetValue()
		}
	}
	return depths
}

func labelValue(metric *dto.Metric, name string) string {
	for _, label := range metric.GetLabel() {
		if label == nil {
			continue
		}
		if strings.EqualFold(label.GetName(), name) {
			return label.GetValue()
		}
	}
	return ""
}

func sanitizeLeafCertificates(ids []string, certificates map[string]deploy.LeafCertificate) []map[string]any {
	out := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		entry := map[string]any{
			"node_id": id,
		}
		if cert, ok := certificates[id]; ok {
			entry["usage"] = cert.Usage
			entry["version"] = cert.Version
			entry["parent_version"] = cert.ParentVersion
			entry["serial_number"] = cert.SerialNumber
			entry["certificate_pem"] = cert.CertificatePEM
			if !cert.IssuedAt.IsZero() {
				entry["issued_at"] = cert.IssuedAt.UTC().Format(time.RFC3339Nano)
			}
			if !cert.ExpiresAt.IsZero() {
				entry["expires_at"] = cert.ExpiresAt.UTC().Format(time.RFC3339Nano)
			}
		} else {
			entry["missing"] = true
		}
		out = append(out, entry)
	}
	return out
}

type signedEnvelope struct {
	Payload   json.RawMessage `json:"payload"`
	Signature signatureDTO    `json:"signature"`
}

type signatureDTO struct {
	Algorithm string `json:"algorithm"`
	KeyID     string `json:"key_id"`
	Value     string `json:"value"`
}

func (s *controlPlaneServer) sendSignedJSON(w http.ResponseWriter, status int, payload any, bundle deploy.CABundle) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode signed payload: %w", err)
	}
	signature, err := signPayload(bundle, data)
	if err != nil {
		return err
	}
	env := signedEnvelope{
		Payload:   data,
		Signature: signature,
	}
	writeJSON(w, status, env)
	return nil
}

func signPayload(bundle deploy.CABundle, payload []byte) (signatureDTO, error) {
	key, err := parsePrivateKey(bundle.KeyPEM)
	if err != nil {
		return signatureDTO{}, err
	}
	digest := sha256.Sum256(payload)
	sig, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	if err != nil {
		return signatureDTO{}, fmt.Errorf("sign payload: %w", err)
	}
	return signatureDTO{
		Algorithm: "ES256",
		KeyID:     strings.TrimSpace(bundle.Version),
		Value:     base64.StdEncoding.EncodeToString(sig),
	}, nil
}

func parsePrivateKey(pemData string) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, errors.New("beacon: decode private key")
	}
	if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("beacon: parse private key: %w", err)
	}
	ecdsaKey, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("beacon: private key is not ECDSA")
	}
	return ecdsaKey, nil
}
