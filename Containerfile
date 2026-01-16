FROM --platform=$BUILDPLATFORM registry.access.redhat.com/ubi9/go-toolset:1.22 AS builder

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
ARG GIT_TREE_STATE=unknown

USER root
WORKDIR /src
ENV GOTOOLCHAIN=auto

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

FROM registry.access.redhat.com/ubi9-micro:latest

COPY --from=builder /fray /fray

EXPOSE 5000

ENTRYPOINT ["/fray"]
CMD ["proxy", "-l", ":5000", "-d", "/data"]
