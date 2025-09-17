package build

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	orchestration "github.com/iw2rmb/ploy/internal/orchestration"
)

type hmMock struct{ mock.Mock }

func (m *hmMock) GetJobStatus(jobID string) (*orchestration.JobStatus, error) {
	args := m.Called(jobID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*orchestration.JobStatus), args.Error(1)
}
func (m *hmMock) GetJobAllocations(jobID string) ([]*orchestration.AllocationStatus, error) {
	args := m.Called(jobID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*orchestration.AllocationStatus), args.Error(1)
}

func TestGetLogsWithMonitor_ValidationAndNotFound(t *testing.T) {
	app := fiber.New()
	m := &hmMock{}
	app.Get("/apps/:app/logs", func(c *fiber.Ctx) error { return getLogsWithMonitor(c, m) })

	req := httptest.NewRequest("GET", "/apps/"+url.PathEscape("invalid app")+"/logs", nil)
	resp, err := app.Test(req, 5000)
	require.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)

	// For missing app, all lanes return not found
	m.ExpectedCalls = nil
	m.On("GetJobStatus", mock.Anything).Return(nil, fmt.Errorf("not found"))
	req = httptest.NewRequest("GET", "/apps/missing/logs", nil)
	resp, err = app.Test(req, 5000)
	require.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestGetLogsWithMonitor_SuccessPath(t *testing.T) {
	app := fiber.New()
	m := &hmMock{}
	m.On("GetJobStatus", "svc-lane-a").Return(&orchestration.JobStatus{ID: "id", Name: "svc-lane-a", Status: "running"}, nil)
	healthy := true
	allocs := []*orchestration.AllocationStatus{{ID: "alloc-1", ClientStatus: "running", DesiredStatus: "run", DeploymentStatus: &orchestration.AllocDeploymentStatus{Healthy: &healthy, Timestamp: time.Now().Format(time.RFC3339)}}}
	m.On("GetJobAllocations", "svc-lane-a").Return(allocs, nil)

	app.Get("/apps/:app/logs", func(c *fiber.Ctx) error { return getLogsWithMonitor(c, m) })

	u := fmt.Sprintf("/apps/%s/logs?lines=5", url.PathEscape("svc"))
	req := httptest.NewRequest("GET", u, nil)
	resp, err := app.Test(req, 5000)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "svc", body["app_name"])
	assert.Equal(t, "svc-lane-a", body["job_name"])
	assert.Equal(t, "5", body["lines_requested"])
	_, ok := body["timestamp"]
	assert.True(t, ok)
}

func TestGetLogs_WiresRealMonitorFunction(t *testing.T) {
	app := fiber.New()
	app.Get("/apps/:app/logs", GetLogs)

	req := httptest.NewRequest("GET", "/apps/"+url.PathEscape("invalid app")+"/logs", nil)
	resp, err := app.Test(req, 5000)
	require.NoError(t, err)
	// function executed and returned validation error
	assert.Equal(t, 400, resp.StatusCode)
}
