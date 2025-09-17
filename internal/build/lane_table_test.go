package build

import (
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/internal/config"
	project "github.com/iw2rmb/ploy/internal/detect/project"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestBuildLaneAB_Table(t *testing.T) {
	original := unikraftBuilder
	t.Cleanup(func() { unikraftBuilder = original })

	tmp := t.TempDir()

	tests := []struct {
		name    string
		stub    func(app, lane, srcDir, sha, outDir string, envVars map[string]string) (string, error)
		wantErr bool
	}{
		{
			name: "success",
			stub: func(app, lane, srcDir, sha, outDir string, envVars map[string]string) (string, error) {
				return filepath.Join(outDir, "unikraft.img"), nil
			},
		},
		{
			name: "builder error",
			stub: func(app, lane, srcDir, sha, outDir string, envVars map[string]string) (string, error) {
				return "", errors.New("unikraft failure")
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			unikraftBuilder = tc.stub
			img, err := buildLaneAB(nil, &BuildDependencies{}, "app", "A", tmp, "sha", tmp, map[string]string{})
			if tc.wantErr {
				require.Error(t, err)
				require.Empty(t, img)
			} else {
				require.NoError(t, err)
				require.Equal(t, filepath.Join(tmp, "unikraft.img"), img)
			}
		})
	}
}

func TestBuildLaneD_Table(t *testing.T) {
	original := jailBuilder
	t.Cleanup(func() { jailBuilder = original })

	tmp := t.TempDir()

	tests := []struct {
		name    string
		stub    func(app, srcDir, sha, outDir string, envVars map[string]string) (string, error)
		wantErr bool
	}{
		{
			name: "success",
			stub: func(app, srcDir, sha, outDir string, envVars map[string]string) (string, error) {
				return filepath.Join(outDir, "jail.img"), nil
			},
		},
		{
			name: "builder error",
			stub: func(app, srcDir, sha, outDir string, envVars map[string]string) (string, error) {
				return "", errors.New("jail failure")
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			jailBuilder = tc.stub
			img, err := buildLaneD("app", tmp, "sha", tmp, map[string]string{})
			if tc.wantErr {
				require.Error(t, err)
				require.Empty(t, img)
			} else {
				require.NoError(t, err)
				require.Equal(t, filepath.Join(tmp, "jail.img"), img)
			}
		})
	}
}

func TestBuildLaneF_Table(t *testing.T) {
	original := vmBuilder
	t.Cleanup(func() { vmBuilder = original })

	tmp := t.TempDir()

	tests := []struct {
		name    string
		stub    func(app, sha, outDir string, envVars map[string]string) (string, error)
		wantErr bool
	}{
		{
			name: "success",
			stub: func(app, sha, outDir string, envVars map[string]string) (string, error) {
				return filepath.Join(outDir, "vm.img"), nil
			},
		},
		{
			name: "builder error",
			stub: func(app, sha, outDir string, envVars map[string]string) (string, error) {
				return "", errors.New("vm failure")
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			vmBuilder = tc.stub
			img, err := buildLaneF("app", "sha", tmp, map[string]string{})
			if tc.wantErr {
				require.Error(t, err)
				require.Empty(t, img)
			} else {
				require.NoError(t, err)
				require.Equal(t, filepath.Join(tmp, "vm.img"), img)
			}
		})
	}
}

func TestBuildLaneE_JibPaths(t *testing.T) {
	original := ociBuilder
	t.Cleanup(func() { ociBuilder = original })

	tmp := t.TempDir()
	deps := &BuildDependencies{}
	buildCtx := &BuildContext{APIContext: "apps", AppType: config.UserApp}
	facts := project.BuildFacts{HasJib: true}

	tests := []struct {
		name        string
		stub        func(app, srcDir, tag string, envVars map[string]string) (string, error)
		wantStatus  int
		expectImage string
	}{
		{
			name: "success",
			stub: func(app, srcDir, tag string, envVars map[string]string) (string, error) {
				return "registry.example.com/app:sha", nil
			},
			wantStatus:  200,
			expectImage: "registry.example.com/app:sha",
		},
		{
			name: "missing prerequisites",
			stub: func(app, srcDir, tag string, envVars map[string]string) (string, error) {
				return "", errors.New("OCI build failed: no dockerfile or jib")
			},
			wantStatus:  400,
			expectImage: "",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ociBuilder = tc.stub

			app := fiber.New()
			var (
				imagePath   string
				dockerImage string
				err         error
			)
			app.Post("/lane-e", func(c *fiber.Ctx) error {
				imagePath, dockerImage, _, err = buildLaneE(c, deps, buildCtx, "app", tmp, "sha", tmp, "", facts, map[string]string{})
				if err != nil {
					return err
				}
				return c.JSON(fiber.Map{"image": dockerImage})
			})

			req := httptest.NewRequest("POST", "/lane-e", nil)
			resp, reqErr := app.Test(req, 1000)
			require.NoError(t, reqErr)
			t.Cleanup(func() {
				if resp != nil && resp.Body != nil {
					_ = resp.Body.Close()
				}
			})

			require.Equal(t, tc.wantStatus, resp.StatusCode)
			require.Empty(t, imagePath)
			require.Equal(t, tc.expectImage, dockerImage)
			if tc.wantStatus < 400 {
				require.NoError(t, err)
			} else {
				require.Nil(t, err)
			}
		})
	}
}

func TestBuildLaneG_Table(t *testing.T) {
	t.Setenv("PLOY_WASM_DISTROLESS", "0")

	tests := []struct {
		name       string
		setup      func(t *testing.T, deps *BuildDependencies, dir string)
		wantStatus int
		expectPath bool
	}{
		{
			name: "existing wasm uploads",
			setup: func(t *testing.T, deps *BuildDependencies, dir string) {
				wasm := filepath.Join(dir, "module.wasm")
				require.NoError(t, os.WriteFile(wasm, []byte("wasm"), 0600))

				mockStorage := new(MockUnifiedStorage)
				mockStorage.On("Put", mock.Anything, "builds/app/sha/module.wasm", mock.Anything, mock.Anything).Return(nil).Once()
				deps.Storage = mockStorage
			},
			wantStatus: 200,
			expectPath: true,
		},
		{
			name: "storage missing returns error",
			setup: func(t *testing.T, deps *BuildDependencies, dir string) {
				wasm := filepath.Join(dir, "module.wasm")
				require.NoError(t, os.WriteFile(wasm, []byte("wasm"), 0600))
				deps.Storage = nil
			},
			wantStatus: 500,
			expectPath: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			deps := &BuildDependencies{}
			dir := t.TempDir()
			if tc.setup != nil {
				setupFn := tc.setup
				setupFn(t, deps, dir)
			}

			app := fiber.New()
			var (
				path string
				err  error
			)
			app.Post("/lane-g", func(c *fiber.Ctx) error {
				path, err = buildLaneG(c, deps, "app", dir, "sha")
				if err != nil {
					return err
				}
				return c.JSON(fiber.Map{"path": path})
			})

			req := httptest.NewRequest("POST", "/lane-g", nil)
			resp, reqErr := app.Test(req, 1000)
			require.NoError(t, reqErr)
			t.Cleanup(func() {
				if resp != nil && resp.Body != nil {
					_ = resp.Body.Close()
				}
			})

			require.Equal(t, tc.wantStatus, resp.StatusCode)
			if tc.expectPath {
				require.NoError(t, err)
				require.NotEmpty(t, path)
				if mockStorage, ok := deps.Storage.(*MockUnifiedStorage); ok {
					mockStorage.AssertExpectations(t)
				}
			} else {
				require.Nil(t, err)
				require.Empty(t, path)
			}
		})
	}
}
