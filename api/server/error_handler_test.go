package server

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	apperr "github.com/iw2rmb/ploy/internal/errors"
)

func TestErrorHandler_TypedErrorJSON(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: func(c *fiber.Ctx, err error) error {
		te := apperr.From(err)
		return c.Status(te.HTTPStatus).JSON(fiber.Map{"error": fiber.Map{"code": te.Code, "message": te.Message}})
	}})
	app.Get("/boom", func(c *fiber.Ctx) error {
		return apperr.NotFound("recipe not found", nil)
	})
	req := httptest.NewRequest("GET", "/boom", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request err: %v", err)
	}
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestErrorHandler_WrapUnknownAsInternal(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: func(c *fiber.Ctx, err error) error {
		te := apperr.From(err)
		return c.Status(te.HTTPStatus).JSON(fiber.Map{"error": fiber.Map{"code": te.Code}})
	}})
	app.Get("/boom", func(c *fiber.Ctx) error { return errors.New("x") })
	req := httptest.NewRequest("GET", "/boom", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request err: %v", err)
	}
	if resp.StatusCode != 500 {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
}
