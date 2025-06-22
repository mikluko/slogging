package otel

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

func Test_OtelHandler(t *testing.T) {
	t.Run("logs with valid span context should include trace_id and span_id", func(t *testing.T) {
		// Create a tracer provider and tracer with service name
		res, err := resource.New(context.Background(),
			resource.WithAttributes(
				semconv.ServiceName("test-service"),
			),
		)
		if err != nil {
			t.Fatalf("failed to create resource: %v", err)
		}

		tp := sdktrace.NewTracerProvider(
			sdktrace.WithResource(res),
		)
		tracer := tp.Tracer("test-tracer")

		// Create capture stream and base handler
		buf := new(bytes.Buffer)
		baseHandler := slog.NewTextHandler(buf, nil)

		// Wrap with OtelHandler
		handler := Wrap(baseHandler)
		logger := slog.New(handler)

		// Start a span and log within its context
		ctx, span := tracer.Start(context.Background(), "test-span")
		defer span.End()

		// Log with the span context
		logger.InfoContext(ctx, "test message with tracing")

		// Get the output (trim trailing newline for splitting)
		output := strings.TrimSuffix(buf.String(), "\n")
		lines := strings.Split(output, "\n")

		// Verify log output
		if len(lines) != 1 {
			t.Fatalf("expected 1 line logged, got: %d", len(lines))
		}

		// Extract span context to get expected values
		spanContext := span.SpanContext()
		expectedTraceID := spanContext.TraceID().String()
		expectedSpanID := spanContext.SpanID().String()

		// Check that otel.trace_id, otel.span_id, and otel.service_name are present in the output
		if !strings.Contains(lines[0], `otel.trace_id=`+expectedTraceID) {
			t.Errorf("expected otel.trace_id=%s in output, got: %s", expectedTraceID, lines[0])
		}
		if !strings.Contains(lines[0], `otel.span_id=`+expectedSpanID) {
			t.Errorf("expected otel.span_id=%s in output, got: %s", expectedSpanID, lines[0])
		}
		if !strings.Contains(lines[0], `otel.service_name=test-service`) {
			t.Errorf("expected otel.service_name=test-service in output, got: %s", lines[0])
		}
	})

	t.Run("logs without span context should not include trace_id and span_id", func(t *testing.T) {
		// Create capture stream and base handler
		buf := new(bytes.Buffer)
		baseHandler := slog.NewTextHandler(buf, nil)

		// Wrap with OtelHandler
		handler := Wrap(baseHandler)
		logger := slog.New(handler)

		// Log without span context
		logger.Info("test message without tracing")

		// Get the output (trim trailing newline for splitting)
		output := strings.TrimSuffix(buf.String(), "\n")
		lines := strings.Split(output, "\n")

		// Verify log output
		if len(lines) != 1 {
			t.Fatalf("expected 1 line logged, got: %d", len(lines))
		}

		// Check that trace_id and span_id are NOT present
		if strings.Contains(lines[0], `otel.trace_id=`) {
			t.Errorf("unexpected otel.trace_id in output: %s", lines[0])
		}
		if strings.Contains(lines[0], `otel.span_id=`) {
			t.Errorf("unexpected otel.span_id in output: %s", lines[0])
		}
	})

	t.Run("logs with invalid span context should not include trace_id and span_id", func(t *testing.T) {
		// Create capture stream and base handler
		buf := new(bytes.Buffer)
		baseHandler := slog.NewTextHandler(buf, nil)

		// Wrap with OtelHandler
		handler := Wrap(baseHandler)
		logger := slog.New(handler)

		// Create a context with an invalid span (no-op span)
		ctx := trace.ContextWithSpan(context.Background(), trace.SpanFromContext(context.Background()))

		// Log with invalid span context
		logger.InfoContext(ctx, "test message with invalid span")

		// Get the output (trim trailing newline for splitting)
		output := strings.TrimSuffix(buf.String(), "\n")
		lines := strings.Split(output, "\n")

		// Verify log output
		if len(lines) != 1 {
			t.Fatalf("expected 1 line logged, got: %d", len(lines))
		}

		// Check that trace_id and span_id are NOT present
		if strings.Contains(lines[0], `otel.trace_id=`) {
			t.Errorf("unexpected otel.trace_id in output: %s", lines[0])
		}
		if strings.Contains(lines[0], `otel.span_id=`) {
			t.Errorf("unexpected otel.span_id in output: %s", lines[0])
		}
	})

	t.Run("trace attributes should be at root level with groups", func(t *testing.T) {
		// Create a tracer provider and tracer with service name
		res, err := resource.New(context.Background(),
			resource.WithAttributes(
				semconv.ServiceName("grouped-service"),
			),
		)
		if err != nil {
			t.Fatalf("failed to create resource: %v", err)
		}

		tp := sdktrace.NewTracerProvider(
			sdktrace.WithResource(res),
		)
		tracer := tp.Tracer("test-tracer")

		// Create capture stream and base handler
		buf := new(bytes.Buffer)
		baseHandler := slog.NewTextHandler(buf, nil)

		// Wrap with OtelHandler
		handler := Wrap(baseHandler)
		logger := slog.New(handler)

		// Create logger with group
		groupedLogger := logger.WithGroup("mygroup")

		// Start a span and log within its context
		ctx, span := tracer.Start(context.Background(), "test-span")
		defer span.End()

		// Log with the span context using grouped logger
		groupedLogger.InfoContext(ctx, "test message", "key", "value")

		// Get the output (trim trailing newline for splitting)
		output := strings.TrimSuffix(buf.String(), "\n")
		lines := strings.Split(output, "\n")

		// Verify log output
		if len(lines) != 1 {
			t.Fatalf("expected 1 line logged, got: %d", len(lines))
		}

		// Verify trace_id, span_id, and service_name are present at root level (otel.* format)
		if !strings.Contains(lines[0], `otel.trace_id=`) {
			t.Errorf("otel.trace_id missing from output: %s", lines[0])
		}
		if !strings.Contains(lines[0], `otel.span_id=`) {
			t.Errorf("otel.span_id missing from output: %s", lines[0])
		}
		if !strings.Contains(lines[0], `otel.service_name=grouped-service`) {
			t.Errorf("otel.service_name missing from output: %s", lines[0])
		}

		// Verify the grouped attribute is present with dot notation
		if !strings.Contains(lines[0], `mygroup.key=value`) {
			t.Errorf("mygroup.key=value missing from output: %s", lines[0])
		}

		// Verify that trace attributes are at root level (not inside mygroup)
		// This is confirmed by the fact that they use otel.trace_id format, not mygroup.otel.trace_id
		// If they were inside mygroup, they would appear as mygroup.otel.trace_id
		if strings.Contains(lines[0], `mygroup.otel.trace_id=`) {
			t.Errorf("trace_id should NOT be inside mygroup, but found mygroup.otel.trace_id in output: %s", lines[0])
		}
		if strings.Contains(lines[0], `mygroup.otel.span_id=`) {
			t.Errorf("span_id should NOT be inside mygroup, but found mygroup.otel.span_id in output: %s", lines[0])
		}
	})

	t.Run("service name should be included when available", func(t *testing.T) {
		// Create tracer provider with service name
		res, err := resource.New(context.Background(),
			resource.WithAttributes(
				semconv.ServiceName("my-service"),
			),
		)
		if err != nil {
			t.Fatalf("failed to create resource: %v", err)
		}

		tp := sdktrace.NewTracerProvider(
			sdktrace.WithResource(res),
		)
		tracer := tp.Tracer("test-tracer")

		buf := new(bytes.Buffer)
		baseHandler := slog.NewTextHandler(buf, nil)
		handler := Wrap(baseHandler)
		logger := slog.New(handler)

		ctx, span := tracer.Start(context.Background(), "test-span")
		defer span.End()

		logger.InfoContext(ctx, "test message")

		output := strings.TrimSuffix(buf.String(), "\n")
		lines := strings.Split(output, "\n")

		if len(lines) != 1 {
			t.Fatalf("expected 1 line logged, got: %d", len(lines))
		}

		if !strings.Contains(lines[0], `otel.service_name=my-service`) {
			t.Errorf("expected otel.service_name=my-service in output, got: %s", lines[0])
		}
	})

	t.Run("service name should not be included when using default unknown service", func(t *testing.T) {
		// Create tracer provider without explicit service name (will use default unknown_service)
		tp := sdktrace.NewTracerProvider()
		tracer := tp.Tracer("test-tracer")

		buf := new(bytes.Buffer)
		baseHandler := slog.NewTextHandler(buf, nil)
		handler := Wrap(baseHandler)
		logger := slog.New(handler)

		ctx, span := tracer.Start(context.Background(), "test-span")
		defer span.End()

		logger.InfoContext(ctx, "test message")

		output := strings.TrimSuffix(buf.String(), "\n")
		lines := strings.Split(output, "\n")

		if len(lines) != 1 {
			t.Fatalf("expected 1 line logged, got: %d", len(lines))
		}

		// Should have trace_id and span_id but not service_name (since it's unknown_service)
		if !strings.Contains(lines[0], `otel.trace_id=`) {
			t.Errorf("expected otel.trace_id in output, got: %s", lines[0])
		}
		if !strings.Contains(lines[0], `otel.span_id=`) {
			t.Errorf("expected otel.span_id in output, got: %s", lines[0])
		}
		if strings.Contains(lines[0], `otel.service_name=`) {
			t.Errorf("unexpected otel.service_name in output (should skip unknown_service): %s", lines[0])
		}
	})

	t.Run("otel group should be at absolute root level with deeply nested groups", func(t *testing.T) {
		// Create a tracer provider with service name
		res, err := resource.New(context.Background(),
			resource.WithAttributes(
				semconv.ServiceName("deep-service"),
			),
		)
		if err != nil {
			t.Fatalf("failed to create resource: %v", err)
		}

		tp := sdktrace.NewTracerProvider(
			sdktrace.WithResource(res),
		)
		tracer := tp.Tracer("test-tracer")

		// Create capture stream and base handler
		buf := new(bytes.Buffer)
		baseHandler := slog.NewTextHandler(buf, nil)

		// Wrap with OtelHandler
		handler := Wrap(baseHandler)
		logger := slog.New(handler)

		// Create deeply nested groups: consumer.checkconfig.scheduler.consumer.checkconfig
		deepLogger := logger.
			WithGroup("consumer").
			WithGroup("checkconfig").
			WithGroup("scheduler").
			WithGroup("consumer").
			WithGroup("checkconfig")

		// Start a span and log within its context
		ctx, span := tracer.Start(context.Background(), "test-span")
		defer span.End()

		// Log with the span context using deeply nested logger
		deepLogger.InfoContext(ctx, "test message", "key", "value")

		// Get the output (trim trailing newline for splitting)
		output := strings.TrimSuffix(buf.String(), "\n")
		lines := strings.Split(output, "\n")

		// Verify log output
		if len(lines) != 1 {
			t.Fatalf("expected 1 line logged, got: %d", len(lines))
		}

		// Verify otel attributes are at absolute root level (otel.* format)
		if !strings.Contains(lines[0], `otel.trace_id=`) {
			t.Errorf("otel.trace_id missing from output: %s", lines[0])
		}
		if !strings.Contains(lines[0], `otel.span_id=`) {
			t.Errorf("otel.span_id missing from output: %s", lines[0])
		}
		if !strings.Contains(lines[0], `otel.service_name=deep-service`) {
			t.Errorf("otel.service_name missing from output: %s", lines[0])
		}

		// Verify the nested grouped attribute is present with dot notation
		if !strings.Contains(lines[0], `consumer.checkconfig.scheduler.consumer.checkconfig.key=value`) {
			t.Errorf("nested grouped attribute missing from output: %s", lines[0])
		}

		// Verify that otel group is NOT nested inside any other groups
		// It should NOT appear as consumer.otel.*, consumer.checkconfig.otel.*, etc.
		if strings.Contains(lines[0], `consumer.otel.trace_id=`) {
			t.Errorf("otel group should NOT be inside consumer group, but found consumer.otel.trace_id in output: %s", lines[0])
		}
		if strings.Contains(lines[0], `consumer.checkconfig.otel.trace_id=`) {
			t.Errorf("otel group should NOT be inside consumer.checkconfig group, but found consumer.checkconfig.otel.trace_id in output: %s", lines[0])
		}
		if strings.Contains(lines[0], `consumer.checkconfig.scheduler.consumer.checkconfig.otel.trace_id=`) {
			t.Errorf("otel group should NOT be inside nested groups, but found nested otel.trace_id in output: %s", lines[0])
		}
	})

	t.Run("WithAttrs should respect groups properly", func(t *testing.T) {
		// Create a tracer provider with service name
		res, err := resource.New(context.Background(),
			resource.WithAttributes(
				semconv.ServiceName("test-attrs-service"),
			),
		)
		if err != nil {
			t.Fatalf("failed to create resource: %v", err)
		}
		
		tp := sdktrace.NewTracerProvider(
			sdktrace.WithResource(res),
		)
		tracer := tp.Tracer("test-tracer")

		// Create capture stream and base handler
		buf := new(bytes.Buffer)
		baseHandler := slog.NewTextHandler(buf, nil)

		// Wrap with OtelHandler
		handler := Wrap(baseHandler)
		logger := slog.New(handler)

		// Create nested groups: module.component
		groupedLogger := logger.WithGroup("module").WithGroup("component")
		
		// Add attributes to the grouped logger (this should put them in module.component group)
		loggerWithAttrs := groupedLogger.With(
			slog.Int("counter", 42),
			slog.String("status", "active"),
			slog.Bool("enabled", true),
		)

		// Start a span and log within its context
		ctx, span := tracer.Start(context.Background(), "test-span")
		defer span.End()

		// Log with the span context
		loggerWithAttrs.InfoContext(ctx, "operation completed successfully")

		// Get the output (trim trailing newline for splitting)
		output := strings.TrimSuffix(buf.String(), "\n")
		lines := strings.Split(output, "\n")

		// Verify log output
		if len(lines) != 1 {
			t.Fatalf("expected 1 line logged, got: %d", len(lines))
		}

		line := lines[0]
		t.Logf("Output: %s", line)

		// Verify otel attributes are at absolute root level
		if !strings.Contains(line, `otel.trace_id=`) {
			t.Errorf("otel.trace_id missing from output: %s", line)
		}
		if !strings.Contains(line, `otel.span_id=`) {
			t.Errorf("otel.span_id missing from output: %s", line)
		}
		if !strings.Contains(line, `otel.service_name=test-attrs-service`) {
			t.Errorf("otel.service_name missing from output: %s", line)
		}

		// Verify the grouped attributes are properly nested in module.component group
		if !strings.Contains(line, `module.component.counter=42`) {
			t.Errorf("module.component.counter missing from output: %s", line)
		}
		if !strings.Contains(line, `module.component.status=active`) {
			t.Errorf("module.component.status missing from output: %s", line)
		}
		if !strings.Contains(line, `module.component.enabled=true`) {
			t.Errorf("module.component.enabled missing from output: %s", line)
		}

		// Verify attributes are NOT at root level (that would be the bug)
		if strings.Contains(line, `counter=42`) && !strings.Contains(line, `module.component.counter=42`) {
			t.Errorf("counter should be in module.component group, not at root: %s", line)
		}
		if strings.Contains(line, `status=active`) && !strings.Contains(line, `module.component.status=active`) {
			t.Errorf("status should be in module.component group, not at root: %s", line)
		}
		if strings.Contains(line, `enabled=true`) && !strings.Contains(line, `module.component.enabled=true`) {
			t.Errorf("enabled should be in module.component group, not at root: %s", line)
		}
	})

	t.Run("groups should work without tracing context", func(t *testing.T) {
		// Create capture stream and base handler
		buf := new(bytes.Buffer)
		baseHandler := slog.NewTextHandler(buf, nil)

		// Wrap with OtelHandler
		handler := Wrap(baseHandler)
		logger := slog.New(handler)

		// Create nested groups and attributes
		groupedLogger := logger.WithGroup("module").WithGroup("component")
		loggerWithAttrs := groupedLogger.With(
			slog.Int("counter", 100),
			slog.String("status", "running"),
		)

		// Log WITHOUT span context (no tracing)
		loggerWithAttrs.Info("operation completed without tracing")

		// Get the output (trim trailing newline for splitting)
		output := strings.TrimSuffix(buf.String(), "\n")
		lines := strings.Split(output, "\n")

		// Verify log output
		if len(lines) != 1 {
			t.Fatalf("expected 1 line logged, got: %d", len(lines))
		}

		line := lines[0]
		t.Logf("Output: %s", line)

		// Verify NO otel attributes (since no tracing)
		if strings.Contains(line, `otel.trace_id=`) {
			t.Errorf("unexpected otel.trace_id in output without tracing: %s", line)
		}
		if strings.Contains(line, `otel.span_id=`) {
			t.Errorf("unexpected otel.span_id in output without tracing: %s", line)
		}

		// Verify the grouped attributes are properly nested in module.component group
		if !strings.Contains(line, `module.component.counter=100`) {
			t.Errorf("module.component.counter missing from output: %s", line)
		}
		if !strings.Contains(line, `module.component.status=running`) {
			t.Errorf("module.component.status missing from output: %s", line)
		}

		// Verify attributes are NOT at root level (that would be the bug)
		if strings.Contains(line, `counter=100`) && !strings.Contains(line, `module.component.counter=100`) {
			t.Errorf("counter should be in module.component group, not at root: %s", line)
		}
		if strings.Contains(line, `status=running`) && !strings.Contains(line, `module.component.status=running`) {
			t.Errorf("status should be in module.component group, not at root: %s", line)
		}
	})
}
