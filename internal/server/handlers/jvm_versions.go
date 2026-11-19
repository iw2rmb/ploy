package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/trinodb/trino-go-client/trino"

	"github.com/iw2rmb/ploy/internal/domain/api"
	"github.com/iw2rmb/ploy/internal/server/deployments"
)

const jvmVersionsLookupTimeout = 60 * time.Second

type JVMVersionsLookup interface {
	Lookup(context.Context, deployments.Request) ([][]string, error)
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

func parseJVMVersionsRequest(pathJVM, rawQuery string) (deployments.Request, error) {
	var request deployments.Request
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
	seenFor := make(map[deployments.Coordinate]bool)
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
		seenWith := make(map[deployments.Coordinate]string)
		for _, item := range strings.Split(items[0], ",") {
			coordinateText, version, found := strings.Cut(strings.TrimSpace(item), "@")
			coordinate, err := parseJVMCoordinate(coordinateText)
			if !found || err != nil || version == "" {
				return request, errors.New("with must contain group:artifact@version constraints")
			}
			if seenFor[coordinate] {
				return request, errors.New("for and with must contain different coordinates")
			}
			if previous, exists := seenWith[coordinate]; exists {
				if previous != version {
					return request, errors.New("with must specify one version per coordinate")
				}
				continue
			}
			constraint := deployments.Constraint{Group: coordinate.Group, Artifact: coordinate.Artifact, Version: version}
			request.With = append(request.With, constraint)
			seenWith[coordinate] = version
		}
	}
	return request, nil
}

func parseJVMCoordinate(value string) (deployments.Coordinate, error) {
	group, artifact, found := strings.Cut(value, ":")
	if !found || group == "" || artifact == "" || strings.ContainsAny(group+artifact, ":@") || strings.ContainsFunc(value, unicode.IsSpace) {
		return deployments.Coordinate{}, errors.New("invalid coordinate")
	}
	return deployments.Coordinate{Group: group, Artifact: artifact}, nil
}
