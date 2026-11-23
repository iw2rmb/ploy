package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trinodb/trino-go-client/trino"

	"github.com/iw2rmb/ploy/internal/domain/api"
	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/server/deployments"
)

func TestJVMVersionsTimeoutSurvivesStalledDriverCancellation(t *testing.T) {
	for _, phase := range []string{"initial rows", "row iteration"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			// Real cancellation budgets exceed the server's global 15-second write timeout.
			var cancellations atomic.Int32
			var upstream *httptest.Server
			upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodPost:
					_ = json.NewEncoder(w).Encode(map[string]any{"id": "timeout-test", "nextUri": upstream.URL + "/first"})
				case http.MethodGet:
					if r.URL.Path != "/first" {
						<-r.Context().Done()
						return
					}
					page := map[string]any{
						"id": "timeout-test", "nextUri": upstream.URL + "/next",
						"columns": []map[string]any{
							{"name": "found", "type": "varchar", "typeSignature": map[string]any{"rawType": "varchar", "arguments": []any{}}},
							{"name": "with", "type": "varchar", "typeSignature": map[string]any{"rawType": "varchar", "arguments": []any{}}},
						},
					}
					if phase == "row iteration" {
						page["data"] = [][]string{{`["a:b@1"]`, `[]`}}
					}
					_ = json.NewEncoder(w).Encode(page)
				case http.MethodDelete:
					cancellations.Add(1)
					<-r.Context().Done()
				}
			}))
			defer upstream.Close()
			db, err := sql.Open("trino", upstream.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			handler := http.NewServeMux()
			handler.HandleFunc("GET /v1/deployments/jvm/{jvm}/versions", jvmVersionsHandler(techRadarJVMVersions, deployments.New(db)))
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx, cancel := context.WithTimeout(r.Context(), 200*time.Millisecond)
				defer cancel()
				handler.ServeHTTP(w, r.WithContext(ctx))
			}))
			server.Config.WriteTimeout = 15 * time.Second
			server.Start()
			defer server.Close()
			client := server.Client()
			client.Timeout = 2*trino.DefaultCancelQueryTimeout + 5*time.Second
			started := time.Now()
			response, err := client.Get(server.URL + "/v1/deployments/jvm/21/versions?for=a:b")
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			elapsed := time.Since(started)
			if err != nil || response.StatusCode != http.StatusGatewayTimeout || string(body) != "Tech Radar lookup timed out\n" {
				t.Fatalf("response = %d %q, %v", response.StatusCode, body, err)
			}
			if cancellations.Load() != 2 {
				t.Fatalf("cancellations = %d, want 2", cancellations.Load())
			}
			if elapsed <= server.Config.WriteTimeout || elapsed >= client.Timeout {
				t.Fatalf("response elapsed = %v; want >15s and <%v", elapsed, client.Timeout)
			}
		})
	}
}

func TestJVMVersionsClientDisconnectCancelsTrino(t *testing.T) {
	// Closing the HTTP request cancels upstream work after a result page has arrived.
	started := make(chan struct{})
	cancelled := make(chan struct{}, 2)
	var upstream *httptest.Server
	upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "cancel-test", "nextUri": upstream.URL + "/next"})
		case http.MethodGet:
			close(started)
			<-r.Context().Done()
		case http.MethodDelete:
			cancelled <- struct{}{}
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer upstream.Close()
	db, err := sql.Open("trino", upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/deployments/jvm/{jvm}/versions", jvmVersionsHandler(techRadarJVMVersions, deployments.New(db)))
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/v1/deployments/jvm/21/versions?for=a:b", nil)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		response, err := server.Client().Do(request)
		if err == nil {
			response.Body.Close()
		}
	}()
	select {
	case <-started:
		cancel()
	case <-ctx.Done():
		t.Fatal("Trino query did not start")
	}
	select {
	case <-cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("client disconnect did not cancel Trino")
	}
	<-done
}

func TestJVMVersionsLiveServiceAccount(t *testing.T) {
	// An explicit opt-in keeps normal unit tests independent of production credentials.
	if os.Getenv("PLOY_TRINO_INTEGRATION") != "1" {
		t.Skip("set PLOY_TRINO_INTEGRATION=1 for live Trino verification")
	}
	user, password := os.Getenv("PLOY_TRINO_USER"), os.Getenv("PLOY_TRINO_PASSWORD")
	if user == "" || password == "" {
		t.Fatal("live verification requires Trino service-account credentials")
	}
	db, err := deployments.Open(user, password)
	if err != nil {
		t.Fatal("initialize live Trino lookup")
	}
	defer db.Close()
	const tokenSecret = "local-techradar-verification"
	authorizer := auth.NewAuthorizer(auth.Options{TokenSecret: tokenSecret})
	defer authorizer.Wait()
	mux := http.NewServeMux()
	mux.Handle("GET /v1/deployments/jvm/{jvm}/versions", authorizer.Middleware(auth.RoleControlPlane)(jvmVersionsHandler(techRadarJVMVersions, deployments.New(db))))
	server := httptest.NewServer(mux)
	defer server.Close()
	server.Client().Timeout = 130 * time.Second
	token, err := auth.GenerateAPIToken(tokenSecret, string(auth.RoleControlPlane), time.Now().Add(5*time.Minute))
	if err != nil {
		t.Fatal("create local authentication token")
	}
	request, err := http.NewRequest(http.MethodGet, server.URL+"/v1/deployments/jvm/21/versions?for=ru.tinkoff.top.core:top-tracing-starter&with=org.springframework.boot:spring-boot@3.3.0", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal("live endpoint request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("live endpoint status = %d", response.StatusCode)
	}
	var result api.JVMVersionsResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil || result.Versions == nil {
		t.Fatal("live endpoint returned an invalid response shape")
	}
	for _, observation := range result.Versions {
		if len(observation.Found) == 0 || len(observation.With) != 1 || observation.With[0] != "org.springframework.boot:spring-boot@3.3.0" {
			t.Fatalf("live endpoint returned invalid observation: %+v", observation)
		}
		for _, found := range observation.Found {
			if !strings.HasPrefix(found, "ru.tinkoff.top.core:top-tracing-starter@") {
				t.Fatalf("live endpoint returned unexpected found coordinate: %q", found)
			}
		}
	}
	t.Logf("live service-account lookup returned %d observations", len(result.Versions))
}
