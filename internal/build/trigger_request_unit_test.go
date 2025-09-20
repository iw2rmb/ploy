package build

import (
	"bytes"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"
)

func TestReadRequestBodyToTar_RawBody(t *testing.T) {
	app := fiber.New()
	app.Post("/build", func(c *fiber.Ctx) error {
		f, err := os.CreateTemp("", "raw-*.tar")
		require.NoError(t, err)
		defer func() { _ = os.Remove(f.Name()); _ = f.Close() }()
		n, err := readRequestBodyToTar(c, f)
		require.NoError(t, err)
		require.Equal(t, int64(7), n)
		return c.SendStatus(200)
	})

	req := httptest.NewRequest("POST", "/build", bytes.NewBufferString("content"))
	req.Header.Set("Content-Type", "application/x-tar")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}

func TestReadRequestBodyToTar_Multipart(t *testing.T) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", "src.tar")
	require.NoError(t, err)
	_, _ = part.Write([]byte("archive"))
	require.NoError(t, mw.Close())

	app := fiber.New()
	app.Post("/build", func(c *fiber.Ctx) error {
		f, err := os.CreateTemp("", "mp-*.tar")
		require.NoError(t, err)
		defer func() { _ = os.Remove(f.Name()); _ = f.Close() }()
		n, err := readRequestBodyToTar(c, f)
		require.NoError(t, err)
		require.Equal(t, int64(len("archive")), n)
		return c.SendStatus(200)
	})

	req := httptest.NewRequest("POST", "/build", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}
