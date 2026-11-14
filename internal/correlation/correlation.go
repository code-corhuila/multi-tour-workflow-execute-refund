// Package correlation carries the X-Correlation-Id of a request through the context, so every participant
// call of a saga can send it and every log line can show it.
package correlation

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"regexp"
)

type key struct{}

var valid = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// Valid says whether id is safe to put in a header and a log line: letters, digits, dot, underscore and hyphen.
func Valid(id string) bool { return valid.MatchString(id) }

func With(ctx context.Context, id string) context.Context { return context.WithValue(ctx, key{}, id) }

// From returns the id in ctx, or "" when there is none.
func From(ctx context.Context) string {
	id, _ := ctx.Value(key{}).(string)
	return id
}

// NewID returns a random version 4 UUID.
func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err) // the system source of randomness is gone: nothing sensible can continue
	}
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// NewLogHandler wraps a slog handler so every record logged with a context that carries a correlation id shows it
// as traceId (the name of the error envelope).
func NewLogHandler(inner slog.Handler) slog.Handler { return logHandler{inner} }

type logHandler struct{ slog.Handler }

func (h logHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := From(ctx); id != "" {
		r.AddAttrs(slog.String("traceId", id))
	}
	return h.Handler.Handle(ctx, r)
}

func (h logHandler) WithAttrs(a []slog.Attr) slog.Handler { return logHandler{h.Handler.WithAttrs(a)} }
func (h logHandler) WithGroup(n string) slog.Handler      { return logHandler{h.Handler.WithGroup(n)} }
