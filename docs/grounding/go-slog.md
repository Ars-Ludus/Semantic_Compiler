# Go slog Grounding

The `semcom` project uses the standard library `log/slog` package for structured logging.

## Core Patterns

### 1. Error Logging
Always include the error itself using the `"err"` key. Provide additional context keys as needed.

```go
slog.Error("descriptive message", "err", err, "context_id", id)
```

**Example from codebase (`semcom_embed/index.go`):**
```go
if _, err := bm.ReadFrom(bytes.NewReader(b)); err != nil {
    slog.Error("deserialize bitmap", "id", id, "err", err)
    return nil, fmt.Errorf("deserialize bitmap %d: %w", id, err)
}
```

### 2. Contextual Attributes
Use consistent keys for common attributes:
- `"err"`: The error object.
- `"memory_id"`: Reference to a specific memory/document ID.
- `"id"`: General identifier.

**Example from codebase (`dashboard/main.go`):**
```go
raw, err := s.store.GetRaw(r.Context(), res.MemoryID)
if err != nil {
    slog.Error("GetRaw", "memory_id", res.MemoryID, "err", err)
    continue
}
```

## Conventions

- **Prefer Structured Logging**: Avoid using `log.Print` or `fmt.Print` for application logs.
- **Message Clarity**: The first argument to `slog` functions should be a static, descriptive string. Dynamic data should be passed as attributes.
- **Import Path**: Always use `"log/slog"`.
