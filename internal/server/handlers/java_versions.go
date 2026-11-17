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
	"github.com/iw2rmb/ploy/internal/server/techradar"
)

const javaVersionsLookupTimeout = 60 * time.Second

type JavaVersionsLookup interface {
	Lookup(context.Context, techradar.Request) ([][]string, error)
}

func javaVersionsHandler(lookup JavaVersionsLookup) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		request, err := parseJavaVersionsRequest(r.URL.RawQuery)
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}
		if lookup == nil {
			writeHTTPError(w, http.StatusServiceUnavailable, "Tech Radar is not configured")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), javaVersionsLookupTimeout)
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
			writeJSON(w, http.StatusOK, api.JavaVersionsResponse{Versions: versions})
		}
	}
}

func parseJavaVersionsRequest(rawQuery string) (techradar.Request, error) {
	var request techradar.Request
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return request, errors.New("invalid query encoding")
	}
	for key, items := range values {
		if key != "for" && key != "with" && key != "jdk" {
			return request, errors.New("unknown query parameter")
		}
		if len(items) != 1 {
			return request, fmt.Errorf("%s must occur once", key)
		}
	}
	jdk := values.Get("jdk")
	if jdk == "" || strings.IndexFunc(jdk, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return request, errors.New("jdk must be a positive integer")
	}
	request.JDK, err = strconv.ParseInt(jdk, 10, 64)
	if err != nil || request.JDK <= 0 {
		return request, errors.New("jdk must be a positive integer")
	}
	seenFor := make(map[techradar.Coordinate]bool)
	for _, item := range strings.Split(values.Get("for"), ",") {
		coordinate, err := parseJavaCoordinate(strings.TrimSpace(item))
		if err != nil {
			return request, errors.New("for must contain group:artifact coordinates")
		}
		if !seenFor[coordinate] {
			request.For = append(request.For, coordinate)
			seenFor[coordinate] = true
		}
	}
	if items, present := values["with"]; present {
		seenWith := make(map[techradar.Constraint]bool)
		for _, item := range strings.Split(items[0], ",") {
			coordinateText, version, found := strings.Cut(strings.TrimSpace(item), "@")
			coordinate, err := parseJavaCoordinate(coordinateText)
			if !found || err != nil || version == "" {
				return request, errors.New("with must contain group:artifact@version constraints")
			}
			constraint := techradar.Constraint{Group: coordinate.Group, Artifact: coordinate.Artifact, Version: version}
			if !seenWith[constraint] {
				request.With = append(request.With, constraint)
				seenWith[constraint] = true
			}
		}
	}
	return request, nil
}

func parseJavaCoordinate(value string) (techradar.Coordinate, error) {
	group, artifact, found := strings.Cut(value, ":")
	if !found || group == "" || artifact == "" || strings.ContainsAny(group+artifact, ":@") || strings.ContainsFunc(value, unicode.IsSpace) {
		return techradar.Coordinate{}, errors.New("invalid coordinate")
	}
	return techradar.Coordinate{Group: group, Artifact: artifact}, nil
}
