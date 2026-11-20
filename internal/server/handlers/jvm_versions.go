package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/trinodb/trino-go-client/trino"

	"github.com/iw2rmb/ploy/internal/domain/api"
	"github.com/iw2rmb/ploy/internal/domain/jvmversions"
)

const jvmVersionsLookupTimeout = 60 * time.Second

type JVMVersionsLookup interface {
	Lookup(context.Context, jvmversions.Request) ([]jvmversions.Observation, error)
}

func jvmVersionsHandler(lookup JVMVersionsLookup) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		request, err := parseJVMVersionsRequest(r.PathValue("jvm"), r.URL.RawQuery)
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}
		if lookup == nil {
			writeHTTPError(w, http.StatusServiceUnavailable, "Tech Radar is not configured")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), jvmVersionsLookupTimeout)
		defer cancel()
		deadline, _ := ctx.Deadline()
		// The driver can synchronously cancel twice before returning a timeout.
		deadline = deadline.Add(2*trino.DefaultCancelQueryTimeout + 5*time.Second)
		if err := http.NewResponseController(w).SetWriteDeadline(deadline); err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to set response deadline")
			return
		}
		versions, err := lookup.Lookup(ctx, request)
		if err == nil && ctx.Err() == nil {
			versions = normalizeJVMVersions(versions)
		}
		switch {
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			writeHTTPError(w, http.StatusGatewayTimeout, "Tech Radar lookup timed out")
		case ctx.Err() != nil:
			return
		case err != nil:
			writeHTTPError(w, http.StatusBadGateway, "Tech Radar lookup failed")
		default:
			writeJSON(w, http.StatusOK, api.JVMVersionsResponse{Versions: versions})
		}
	}
}

func parseJVMVersionsRequest(pathJVM, rawQuery string) (jvmversions.Request, error) {
	var request jvmversions.Request
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return request, errors.New("invalid query encoding")
	}
	for key, items := range values {
		if key != "for" && key != "with" {
			return request, errors.New("unknown query parameter")
		}
		if len(items) != 1 {
			return request, fmt.Errorf("%s must occur once", key)
		}
	}
	if pathJVM == "" || strings.IndexFunc(pathJVM, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return request, errors.New("jvm must be a positive integer")
	}
	request.JVM, err = strconv.ParseInt(pathJVM, 10, 64)
	if err != nil || request.JVM <= 0 {
		return request, errors.New("jvm must be a positive integer")
	}
	seenFor := make(map[jvmversions.Coordinate]bool)
	for _, item := range strings.Split(values.Get("for"), ",") {
		coordinate, err := parseJVMCoordinate(strings.TrimSpace(item))
		if err != nil {
			return request, errors.New("for must contain group:artifact coordinates")
		}
		if !seenFor[coordinate] {
			request.For = append(request.For, coordinate)
			seenFor[coordinate] = true
		}
	}
	if items, present := values["with"]; present {
		seenWith := make(map[jvmversions.Coordinate]jvmversions.Selector)
		for _, item := range strings.Split(items[0], ",") {
			coordinateText, version, versioned := strings.Cut(strings.TrimSpace(item), "@")
			coordinate, err := parseJVMCoordinate(coordinateText)
			if err != nil {
				return request, errors.New("with must contain group:artifact or group:artifact@version constraints")
			}
			selector := jvmversions.Selector{Kind: jvmversions.Any}
			if versioned {
				selector, err = parseJVMVersionSelector(version)
				if err != nil {
					return request, err
				}
			}
			if seenFor[coordinate] {
				return request, errors.New("for and with must contain different coordinates")
			}
			if previous, exists := seenWith[coordinate]; exists {
				if previous != selector {
					return request, errors.New("with must specify one selector per coordinate")
				}
				continue
			}
			constraint := jvmversions.Constraint{Coordinate: coordinate, Selector: selector}
			request.With = append(request.With, constraint)
			seenWith[coordinate] = selector
		}
	}
	return request, nil
}

func parseJVMCoordinate(value string) (jvmversions.Coordinate, error) {
	group, artifact, found := strings.Cut(value, ":")
	if !found || group == "" || artifact == "" || strings.ContainsAny(group+artifact, ":@") || strings.ContainsFunc(value, unicode.IsSpace) {
		return jvmversions.Coordinate{}, errors.New("invalid coordinate")
	}
	return jvmversions.Coordinate{Group: group, Artifact: artifact}, nil
}

func parseJVMVersionSelector(version string) (jvmversions.Selector, error) {
	if version == "" {
		return jvmversions.Selector{}, errors.New("with version must not be empty")
	}
	parts := strings.Split(version, ".")
	if !slices.Contains(parts, "x") {
		return jvmversions.Selector{Kind: jvmversions.Exact, Value: version}, nil
	}
	decimal := func(value string) bool {
		return value != "" && strings.IndexFunc(value, func(r rune) bool { return r < '0' || r > '9' }) < 0
	}
	if len(parts) == 3 && decimal(parts[0]) && parts[2] == "x" {
		if parts[1] == "x" {
			return jvmversions.Selector{Kind: jvmversions.Prefix, Value: parts[0] + "."}, nil
		}
		if decimal(parts[1]) {
			return jvmversions.Selector{Kind: jvmversions.Prefix, Value: parts[0] + "." + parts[1] + "."}, nil
		}
	}
	return jvmversions.Selector{}, errors.New("with masks must have the form M.x.x or M.N.x with decimal components")
}

func normalizeJVMVersions(observations []jvmversions.Observation) []jvmversions.Observation {
	normalize := func(values []string) []string {
		if values == nil {
			return []string{}
		}
		slices.Sort(values)
		return slices.Compact(values)
	}
	if observations == nil {
		return []jvmversions.Observation{}
	}
	for i := range observations {
		observations[i].Found = normalize(observations[i].Found)
		observations[i].With = normalize(observations[i].With)
	}
	compare := func(a, b jvmversions.Observation) int {
		if order := slices.Compare(a.Found, b.Found); order != 0 {
			return order
		}
		return slices.Compare(a.With, b.With)
	}
	slices.SortFunc(observations, compare)
	return slices.CompactFunc(observations, func(a, b jvmversions.Observation) bool { return compare(a, b) == 0 })
}
