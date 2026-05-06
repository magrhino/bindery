FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /telemetry-server ./cmd/telemetry-server

FROM alpine:3.21
RUN apk add --no-cache sqlite && adduser -D -u 1000 app
USER app
COPY --from=builder /telemetry-server /telemetry-server
EXPOSE 8080
ENTRYPOINT ["/telemetry-server"]
