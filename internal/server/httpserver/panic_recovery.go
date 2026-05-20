package httpserver

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"runtime/debug"
)

func withPanicRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.Error("http handler panic recovered",
					"method", r.Method,
					"path", r.URL.Path,
					"panic_type", fmt.Sprintf("%T", recovered),
					"panic_message", panicMessageSafe(recovered),
					"stack", string(debug.Stack()),
				)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r)
	})
}

func panicMessageSafe(recovered any) (msg string) {
	switch v := recovered.(type) {
	case string:
		return v
	case runtime.Error:
		return "runtime panic"
	case error:
		// Avoid calling Error() in panic-recovery logging path; panicking Error()
		// implementations should be fixed at source.
		return fmt.Sprintf("%T", v)
	default:
		return fmt.Sprintf("%T", recovered)
	}
}
