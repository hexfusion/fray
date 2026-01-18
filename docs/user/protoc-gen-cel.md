# protoc-gen-cel

Generates CEL-based transition validation from proto annotations. Complements protovalidate with update-time rules.

## Install

```bash
go install github.com/hexfusion/fray/cmd/protoc-gen-cel@latest
```

## Usage

```yaml
# buf.yaml
deps:
  - buf.build/fray/protoc-gen-cel
```

```yaml
# buf.gen.yaml
plugins:
  - local: protoc-gen-cel
    out: gen
    opt: paths=source_relative
```

## Annotations

```protobuf
import "annotations.proto";

message User {
  // Immutable after creation
  string id = 1 [(cel.field).immutable = true];

  // Must increase on update
  int64 version = 2 [(cel.field).transition = {
    expr: "this > oldSelf"
    message: "version must increase"
  }];

  // State machine
  Status status = 3 [(cel.field).transitions = {
    rules: [
      { from: ["PENDING"], to: ["ACTIVE", "CANCELLED"] },
      { from: ["ACTIVE"], to: ["COMPLETED"] }
    ]
    deny_unlisted: true
  }];

  // Format validation
  string email = 4 [(cel.field).format = EMAIL];
}
```

## Generated API

```go
// Validate field constraints
err := user.Validate()

// Validate transition from old state
err := newUser.ValidateTransition(oldUser)
```

## Formats

`EMAIL`, `URI`, `UUID`, `HOSTNAME`, `IPV4`, `IPV6`, `DNS_LABEL`, `DNS_SUBDOMAIN`, `DATETIME`, `SEMVER`
