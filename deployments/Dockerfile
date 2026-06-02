# syntax=docker/dockerfile:1
FROM golang:1.23-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /bunshin-server ./cmd/bunshin/

# ---- runtime ----
FROM gcr.io/distroless/static-debian12
COPY --from=builder /bunshin-server /bunshin-server
EXPOSE 8080
ENTRYPOINT ["/bunshin-server", "serve"]
