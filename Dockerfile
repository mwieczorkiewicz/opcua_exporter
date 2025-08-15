FROM cgr.dev/chainguard/go AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o opcua_exporter ./cmd/opcua_exporter

FROM scratch
WORKDIR /
COPY --from=builder /build/opcua_exporter /
ENTRYPOINT ["/opcua_exporter"]
