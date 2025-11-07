package types

import (
	"encoding"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// CPUmilli is a CPU quantity expressed in milli‑CPUs (1000m = 1 CPU).
//
// It marshals to/from JSON as a string. Text decoding accepts either
// an integer number of CPUs (e.g., "2") or a milli form with the
// trailing 'm' suffix (e.g., "500m"). Negative values are rejected.
type CPUmilli int64

// Bytes is a storage/memory quantity expressed in bytes.
//
// It marshals to/from JSON as a string. Text decoding accepts a plain
// integer byte count (e.g., "1048576") or common unit suffixes:
//   - decimal powers: K, M, G, T, P (10^3 .. 10^15)
//   - binary powers:  Ki, Mi, Gi, Ti, Pi (2^10 .. 2^50)
//
// An optional trailing 'B' is ignored (e.g., "10GB", "2GiB").
// Negative values and unknown suffixes are rejected.
type Bytes int64

// Errors reported by resource parsers.
var (
	ErrInvalidCPU   = errors.New("invalid cpu")
	ErrInvalidBytes = errors.New("invalid bytes")
	ErrOverflow     = errors.New("overflow")
)

var _ interface {
	encoding.TextMarshaler
	encoding.TextUnmarshaler
} = (*CPUmilli)(nil)

// MarshalText implements encoding.TextMarshaler.
func (v CPUmilli) MarshalText() ([]byte, error) {
	if v < 0 {
		return nil, ErrInvalidCPU
	}
	if v%1000 == 0 {
		return []byte(strconv.FormatInt(int64(v/1000), 10)), nil
	}
	return []byte(fmt.Sprintf("%dm", v)), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (v *CPUmilli) UnmarshalText(b []byte) error {
	s := Normalize(string(b))
	if IsEmpty(s) {
		return ErrEmpty
	}
	// milli form
	if strings.HasSuffix(s, "m") {
		nstr := strings.TrimSuffix(s, "m")
		nstr = strings.TrimSpace(nstr)
		n, err := strconv.ParseInt(nstr, 10, 64)
		if err != nil {
			var ne *strconv.NumError
			if errors.As(err, &ne) && ne.Err == strconv.ErrRange {
				return ErrOverflow
			}
			return ErrInvalidCPU
		}
		if n < 0 {
			return ErrInvalidCPU
		}
		*v = CPUmilli(n)
		return nil
	}
	// whole CPU form (optionally with fractional .XYZ up to 3 digits)
	if strings.ContainsRune(s, '.') {
		// require up to 3 fractional digits to map to millis exactly
		parts := strings.SplitN(s, ".", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return ErrInvalidCPU
		}
		ip, fp := parts[0], parts[1]
		if strings.HasPrefix(ip, "+") || strings.HasPrefix(ip, "-") {
			// forbid signs
			return ErrInvalidCPU
		}
		if len(fp) > 3 {
			return ErrInvalidCPU
		}
		for len(fp) < 3 {
			fp += "0"
		}
		in, err1 := strconv.ParseInt(ip, 10, 64)
		if err1 != nil {
			var ne *strconv.NumError
			if errors.As(err1, &ne) && ne.Err == strconv.ErrRange {
				return ErrOverflow
			}
			return ErrInvalidCPU
		}
		fn, err2 := strconv.ParseInt(fp, 10, 64)
		if err2 != nil {
			return ErrInvalidCPU
		}
		if in < 0 || fn < 0 {
			return ErrInvalidCPU
		}
		// check overflow on in*1000 + fn
		if in > math.MaxInt64/1000 {
			return ErrOverflow
		}
		total := in*1000 + fn
		*v = CPUmilli(total)
		return nil
	}
	// plain integer CPUs
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		var ne *strconv.NumError
		if errors.As(err, &ne) && ne.Err == strconv.ErrRange {
			return ErrOverflow
		}
		return ErrInvalidCPU
	}
	if n < 0 {
		return ErrInvalidCPU
	}
	if n > math.MaxInt64/1000 {
		return ErrOverflow
	}
	*v = CPUmilli(n * 1000)
	return nil
}

// MarshalJSON encodes the value as a JSON string.
func (v CPUmilli) MarshalJSON() ([]byte, error) { return MarshalJSONFromText(v) }

// UnmarshalJSON decodes the value from a JSON string.
func (v *CPUmilli) UnmarshalJSON(b []byte) error { return UnmarshalJSONToText(b, v) }

// Validate implements Validatable.
func (v CPUmilli) Validate() error {
	if v < 0 {
		return ErrInvalidCPU
	}
	return nil
}

// DockerNanoCPUs returns the Docker NanoCPUs value (1e9 per CPU).
func (v CPUmilli) DockerNanoCPUs() int64 { return int64(v) * 1_000_000 }

var _ interface {
	encoding.TextMarshaler
	encoding.TextUnmarshaler
} = (*Bytes)(nil)

// MarshalText implements encoding.TextMarshaler. It renders the raw byte count.
func (v Bytes) MarshalText() ([]byte, error) {
	if v < 0 {
		return nil, ErrInvalidBytes
	}
	return []byte(strconv.FormatInt(int64(v), 10)), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (v *Bytes) UnmarshalText(b []byte) error {
	s := Normalize(string(b))
	if IsEmpty(s) {
		return ErrEmpty
	}
	// Strip a single optional trailing 'B'/'b'.
	if len(s) > 1 && (s[len(s)-1] == 'B' || s[len(s)-1] == 'b') {
		s = s[:len(s)-1]
	}
	sTrim := s
	sLower := strings.ToLower(sTrim)

	// Determine suffix and multiplier.
	var mult int64 = 1
	switch {
	case strings.HasSuffix(sLower, "ki"):
		mult = 1 << 10
		sTrim = sTrim[:len(sTrim)-2]
	case strings.HasSuffix(sLower, "mi"):
		mult = 1 << 20
		sTrim = sTrim[:len(sTrim)-2]
	case strings.HasSuffix(sLower, "gi"):
		mult = 1 << 30
		sTrim = sTrim[:len(sTrim)-2]
	case strings.HasSuffix(sLower, "ti"):
		mult = 1 << 40
		sTrim = sTrim[:len(sTrim)-2]
	case strings.HasSuffix(sLower, "pi"):
		mult = 1 << 50
		sTrim = sTrim[:len(sTrim)-2]
	case strings.HasSuffix(sLower, "k"):
		mult = 1_000
		sTrim = sTrim[:len(sTrim)-1]
	case strings.HasSuffix(sLower, "m"):
		mult = 1_000_000
		sTrim = sTrim[:len(sTrim)-1]
	case strings.HasSuffix(sLower, "g"):
		mult = 1_000_000_000
		sTrim = sTrim[:len(sTrim)-1]
	case strings.HasSuffix(sLower, "t"):
		mult = 1_000_000_000_000
		sTrim = sTrim[:len(sTrim)-1]
	case strings.HasSuffix(sLower, "p"):
		mult = 1_000_000_000_000_000
		sTrim = sTrim[:len(sTrim)-1]
	}

	// Number part must be a non-negative base-10 integer.
	sNum := strings.TrimSpace(sTrim)
	if sNum == "" {
		return ErrInvalidBytes
	}
	n, err := strconv.ParseInt(sNum, 10, 64)
	if err != nil {
		var ne *strconv.NumError
		if errors.As(err, &ne) && ne.Err == strconv.ErrRange {
			return ErrOverflow
		}
		return ErrInvalidBytes
	}
	if n < 0 {
		return ErrInvalidBytes
	}
	// Check overflow on multiplication.
	if mult != 0 && n > math.MaxInt64/mult {
		return ErrOverflow
	}
	*v = Bytes(n * mult)
	return nil
}

// MarshalJSON encodes the value as a JSON string of the raw byte count.
func (v Bytes) MarshalJSON() ([]byte, error) { return MarshalJSONFromText(v) }

// UnmarshalJSON decodes the value from a JSON string.
func (v *Bytes) UnmarshalJSON(b []byte) error { return UnmarshalJSONToText(b, v) }

// Validate implements Validatable.
func (v Bytes) Validate() error {
	if v < 0 {
		return ErrInvalidBytes
	}
	return nil
}

// DockerMemoryBytes returns the raw bytes to set Docker memory limits.
func (v Bytes) DockerMemoryBytes() int64 { return int64(v) }
