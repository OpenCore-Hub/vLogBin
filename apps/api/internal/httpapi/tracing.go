package httpapi

import (
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/semconv/v1.34.0"
	"go.opentelemetry.io/otel/trace"
)

// tracingMiddleware starts a span per request named after the matched chi
// route (e.g. "/v1/operator/providers"), so trace views group by endpoint
// rather than by raw URL with query strings. The W3C Trace-Context header of
// an inbound call is extracted by the global propagator, chaining the span
// into the caller's trace; the status code is recorded when the handler
// finishes, and 5xx responses are marked as errors.
//
// The middleware runs outermost (before recovery/ratelimit) so it also
// captures requests that fail early or panic — with the no-op tracer, which
// is the default, all of this is a no-op with negligible overhead.
func (s *Server) tracingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route := r.URL.Path
		if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePattern() != "" {
			route = rc.RoutePattern()
		}
		ctx, span := otel.Tracer("vlogbin.httpapi").Start(r.Context(), route,
			trace.WithAttributes(
				semconv.HTTPRequestMethodKey.String(r.Method),
				semconv.URLPath(r.URL.Path),
				semconv.ClientAddress(hostOnly(r.RemoteAddr)),
			),
		)
		defer span.End()

		sr := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sr, r.WithContext(ctx))

		// chi middleware runs before route matching, so the route pattern is
		// only known after the handler completed; rename the span so trace
		// views group by endpoint rather than raw URL.
		if rc := chi.RouteContext(ctx); rc != nil && rc.RoutePattern() != "" {
			span.SetName(rc.RoutePattern())
		}
		span.SetAttributes(semconv.HTTPResponseStatusCode(sr.status))
		if sr.status >= 500 {
			span.SetStatus(codes.Error, "HTTP "+strconv.Itoa(sr.status))
		}
	})
}

// hostOnly strips the port from an "ip:port" RemoteAddr so client.address
// carries a clean value.
func hostOnly(addr string) string {
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return strings.TrimSpace(addr)
}
