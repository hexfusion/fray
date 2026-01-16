FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS builder

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
ARG GIT_TREE_STATE=unknown

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags "-s -w \
        -X github.com/hexfusion/fray/pkg/version.version=${VERSION} \
        -X github.com/hexfusion/fray/pkg/version.commit=${COMMIT} \
        -X github.com/hexfusion/fray/pkg/version.buildDate=${BUILD_DATE} \
        -X github.com/hexfusion/fray/pkg/version.gitTreeState=${GIT_TREE_STATE}" \
    -o /fray ./cmd/fray

FROM scratch

COPY --from=builder /fray /fray
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

EXPOSE 5000

ENTRYPOINT ["/fray"]
CMD ["proxy", "-l", ":5000", "-d", "/data"]
