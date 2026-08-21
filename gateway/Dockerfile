# ------------------------------------------------------------------------------
# Build Stage
# ------------------------------------------------------------------------------
FROM golang:alpine AS builder

WORKDIR /build

ENV GOTOOLCHAIN=auto
RUN apk add --no-cache git ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build stripped static binaries with zero CGO
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /build/bin/gargoyle ./cmd/gargoyle
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /build/bin/dummybackend ./cmd/dummybackend

# ------------------------------------------------------------------------------
# Production Runtime: Gargoyle Gateway
# ------------------------------------------------------------------------------
FROM alpine:3.20 AS gargoyle

RUN apk --no-cache add ca-certificates tzdata \
    && addgroup -S gargoyle && adduser -S gargoyle -G gargoyle

WORKDIR /app

COPY --from=builder /build/bin/gargoyle /app/gargoyle
COPY abuse_model.onnx* /app/

RUN chown -R gargoyle:gargoyle /app

USER gargoyle

EXPOSE 8080

ENTRYPOINT ["/app/gargoyle"]

# ------------------------------------------------------------------------------
# Production Runtime: Dummy Mock Upstream Backend
# ------------------------------------------------------------------------------
FROM alpine:3.20 AS dummybackend

RUN apk --no-cache add ca-certificates tzdata \
    && addgroup -S gargoyle && adduser -S gargoyle -G gargoyle

WORKDIR /app

COPY --from=builder /build/bin/dummybackend /app/dummybackend

RUN chown -R gargoyle:gargoyle /app

USER gargoyle

EXPOSE 9000

ENTRYPOINT ["/app/dummybackend"]
