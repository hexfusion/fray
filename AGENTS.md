# Fray Development Principles

## Dependencies

Minimal. "A little copying is better than a little dependency." — Rob Pike

Prefer standard library. When external dependencies are necessary, choose stable, focused packages over feature-rich frameworks.

## Error Handling

Wrap errors with context:
```go
return fmt.Errorf("open config: %w", err)
```

Check errors by type or sentinel, never string matching:
```go
// good
if errors.Is(err, os.ErrNotExist) { ... }

// bad
if strings.Contains(err.Error(), "not found") { ... }
```

## Testing

Use `require` only, not `assert`. Fail fast.

```go
require.NoError(t, err)
require.Equal(t, expected, actual)
```

Table-driven tests where reasonable:
```go
tests := []struct {
    name   string
    input  string
    want   int
}{
    {"empty", "", 0},
    {"single", "a", 1},
}

for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        got := Len(tt.input)
        require.Equal(t, tt.want, got)
    })
}
```

## Comments

Inline comments: minimal, lowercase, no punctuation unless multi-sentence.
```go
// check if already exists
if _, err := os.Stat(path); err == nil {
    return nil
}
```

## Documentation

Go doc comments: idiomatic, concise. One line when possible.
```go
// Client fetches OCI artifacts from registries.
type Client struct { ... }

// GetManifest retrieves a manifest by reference.
func (c *Client) GetManifest(ctx context.Context, ref string) (*Manifest, error)
```

## Lifecycle

Graceful termination is bulletproof. Signal-driven (SIGTERM, SIGINT).

- Finish in-flight requests
- Flush logs
- Clean up resources
- Exit cleanly

Never ignore signals. Never force-kill from within.

## Performance

Critical concern. Optimize for:
- Binary size (minimal dependencies, no reflection-heavy frameworks)
- Memory usage (streaming over buffering, reuse allocations)
- CPU (avoid unnecessary work, prefer simple algorithms)

Profile before optimizing. Measure after.

## General

Less is more.
