package types

import (
	"encoding"
	"errors"
	"strings"
)

// Protocol is a network protocol enum with canonical string values "tcp" or "udp".
//
// It trims surrounding spaces and lower-cases on decode. JSON uses string form.
// Validation rejects empty and unknown values.
type Protocol string

// ProtocolTCP is the TCP network protocol.
//
// It is the canonical lowercase form used for marshaling.
const (
	ProtocolTCP Protocol = "tcp"
	// ProtocolUDP is the UDP network protocol in canonical lowercase form.
	ProtocolUDP Protocol = "udp"
)

// ErrInvalidProtocol indicates the protocol is unknown.
var ErrInvalidProtocol = errors.New("invalid protocol")

// String returns the underlying protocol string.
func (p Protocol) String() string { return string(p) }

var _ interface {
	encoding.TextMarshaler
	encoding.TextUnmarshaler
} = (*Protocol)(nil)

// MarshalText implements encoding.TextMarshaler.
func (p Protocol) MarshalText() ([]byte, error) {
	s := strings.ToLower(Normalize(string(p)))
	if IsEmpty(s) {
		return nil, ErrEmpty
	}
	if !allowedProtocol(s) {
		return nil, ErrInvalidProtocol
	}
	return []byte(s), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (p *Protocol) UnmarshalText(b []byte) error {
	s := strings.ToLower(Normalize(string(b)))
	if IsEmpty(s) {
		return ErrEmpty
	}
	if !allowedProtocol(s) {
		return ErrInvalidProtocol
	}
	*p = Protocol(s)
	return nil
}

// MarshalJSON encodes the value as a JSON string.
func (p Protocol) MarshalJSON() ([]byte, error) { return MarshalJSONFromText(p) }

// UnmarshalJSON decodes the value from a JSON string.
func (p *Protocol) UnmarshalJSON(b []byte) error { return UnmarshalJSONToText(b, p) }

// Validate implements Validatable.
func (p Protocol) Validate() error {
	s := strings.ToLower(Normalize(string(p)))
	if IsEmpty(s) {
		return ErrEmpty
	}
	if !allowedProtocol(s) {
		return ErrInvalidProtocol
	}
	return nil
}

func allowedProtocol(s string) bool { return s == string(ProtocolTCP) || s == string(ProtocolUDP) }
