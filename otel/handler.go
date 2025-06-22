package otel

import (
	"context"
	"log/slog"
	"strings"

	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

// getServiceName extracts the service name from the span's resource.
// Returns empty string if service name is not available or is the default unknown service.
func getServiceName(span trace.Span) string {
	if span == nil {
		return ""
	}

	// Get resource from span (available in newer SDK versions)
	if spanWithResource, ok := span.(interface{ Resource() *resource.Resource }); ok {
		if res := spanWithResource.Resource(); res != nil {
			if serviceName, exists := res.Set().Value(semconv.ServiceNameKey); exists {
				name := serviceName.AsString()
				// Skip default unknown service names
				if name != "" && !strings.HasPrefix(name, "unknown_service:") {
					return name
				}
			}
		}
	}

	return ""
}

// Handler is a slog.Handler that adds OpenTelemetry trace context
// (trace_id, span_id, and service_name) to log records. It wraps another handler and
// ensures trace attributes are always added at the root level in an "otel" group.
type Handler struct {
	handler     slog.Handler // Always the original base handler, never wrapped
	preAttrs    []slog.Attr  // Attributes to prepend (including trace attrs)
	groups      []string     // Current group path
	groupedAttrs []slog.Attr // Attributes that should be placed in current group
}

// Wrap creates a new OpenTelemetry-aware handler that wraps
// the provided handler. When a valid span context is present in the
// context passed to logging methods, it automatically adds trace_id,
// span_id, and service_name attributes at the root level in an "otel" group.
func Wrap(handler slog.Handler) *Handler {
	return &Handler{
		handler:      handler,
		preAttrs:     nil,
		groups:       nil,
		groupedAttrs: nil,
	}
}

// Enabled reports whether the handler handles records at the given level.
func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

// Handle processes the Record by adding trace context if present,
// then delegates to the wrapped handler.
func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	// Check for span context
	span := trace.SpanFromContext(ctx)
	if !span.SpanContext().IsValid() && len(h.preAttrs) == 0 && len(h.groups) == 0 && len(h.groupedAttrs) == 0 {
		// No trace context, no pre-attrs, no groups, no grouped attrs - safe to pass through
		return h.handler.Handle(ctx, r)
	}

	// We need to inject attributes at the root level
	// Create a new record with our pre-attrs first
	newRecord := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)

	// Add trace attributes if present
	if span.SpanContext().IsValid() {
		otelAttrs := []any{
			slog.String("trace_id", span.SpanContext().TraceID().String()),
			slog.String("span_id", span.SpanContext().SpanID().String()),
		}

		// Add service name if available
		if serviceName := getServiceName(span); serviceName != "" {
			otelAttrs = append(otelAttrs, slog.String("service_name", serviceName))
		}

		newRecord.AddAttrs(slog.Group("otel", otelAttrs...))
	}

	// Add any pre-attrs
	newRecord.AddAttrs(h.preAttrs...)

	// Now we need to handle groups properly
	// We'll rebuild the structure with groups
	if len(h.groups) > 0 {
		// Collect all attributes that should be in the group:
		// 1. Attributes from the original record (r.Attrs)
		// 2. Attributes added via WithAttrs (h.groupedAttrs)
		var allGroupedAttrs []slog.Attr
		
		// Add grouped attributes first (from WithAttrs calls)
		allGroupedAttrs = append(allGroupedAttrs, h.groupedAttrs...)
		
		// Add attributes from the record
		r.Attrs(func(a slog.Attr) bool {
			allGroupedAttrs = append(allGroupedAttrs, a)
			return true
		})

		// Build nested groups from inside out
		// Convert attrs to any slice
		anyAttrs := make([]any, len(allGroupedAttrs))
		for i, a := range allGroupedAttrs {
			anyAttrs[i] = a
		}

		current := slog.Group(h.groups[len(h.groups)-1], anyAttrs...)
		for i := len(h.groups) - 2; i >= 0; i-- {
			current = slog.Group(h.groups[i], current)
		}

		newRecord.AddAttrs(current)
	} else {
		// No groups, add grouped attrs and record attrs at root level
		newRecord.AddAttrs(h.groupedAttrs...)
		r.Attrs(func(a slog.Attr) bool {
			newRecord.AddAttrs(a)
			return true
		})
	}

	// Use the base handler (not the grouped one)
	return h.handler.Handle(ctx, newRecord)
}

// WithAttrs returns a new Handler that includes the given attributes.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}

	if len(h.groups) == 0 {
		// At root level, add to preAttrs
		newPreAttrs := make([]slog.Attr, len(h.preAttrs)+len(attrs))
		copy(newPreAttrs, h.preAttrs)
		copy(newPreAttrs[len(h.preAttrs):], attrs)

		return &Handler{
			handler:      h.handler,
			preAttrs:     newPreAttrs,
			groups:       h.groups,
			groupedAttrs: h.groupedAttrs,
		}
	}

	// In a group, add to groupedAttrs to be processed during Handle
	newGroupedAttrs := make([]slog.Attr, len(h.groupedAttrs)+len(attrs))
	copy(newGroupedAttrs, h.groupedAttrs)
	copy(newGroupedAttrs[len(h.groupedAttrs):], attrs)

	return &Handler{
		handler:      h.handler, // Always keep the original base handler
		preAttrs:     h.preAttrs,
		groups:       h.groups,
		groupedAttrs: newGroupedAttrs,
	}
}

// WithGroup returns a new Handler that starts a group.
func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}

	newGroups := make([]string, len(h.groups)+1)
	copy(newGroups, h.groups)
	newGroups[len(h.groups)] = name

	// ALWAYS keep the base handler - never call WithGroup on it
	// We'll handle ALL grouping ourselves in Handle to ensure
	// otel attributes stay at the absolute root level
	return &Handler{
		handler:      h.handler, // Always use the original base handler
		preAttrs:     h.preAttrs,
		groups:       newGroups,
		groupedAttrs: h.groupedAttrs, // Carry forward any grouped attributes
	}
}
