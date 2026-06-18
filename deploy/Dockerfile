# Build the pluggable YDB state store component image.
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/daprd-ydb ./cmd/daprd-ydb

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/daprd-ydb /daprd-ydb
# The Dapr sidecar discovers the component via the shared sockets folder.
# Set both the modern (plural, used by daprd/docs) and the SDK (singular, read by
# components-go-sdk v0.3.0) variable names so a custom path stays in sync.
ENV DAPR_COMPONENTS_SOCKETS_FOLDER=/tmp/dapr-components-sockets \
    DAPR_COMPONENT_SOCKETS_FOLDER=/tmp/dapr-components-sockets
ENTRYPOINT ["/daprd-ydb"]
