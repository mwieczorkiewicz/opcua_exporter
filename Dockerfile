FROM golang:1.24.0-bullseye as go

FROM go as tester
COPY . /build
WORKDIR /build
RUN go test

FROM go as builder
COPY --from=tester /build /build
COPY --from=tester /go /go
WORKDIR /build
RUN CGO_ENABLED=0 GOOS=linux go build -o opcua_exporter .

FROM scratch
WORKDIR /
COPY --from=builder /build/opcua_exporter /
ENTRYPOINT ["/opcua_exporter"]
