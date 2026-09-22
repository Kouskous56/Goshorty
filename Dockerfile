FROM golang:1.26.5-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG VERSION=3.0.0
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o /out/goshorty .

FROM alpine:3.22
RUN apk add --no-cache ca-certificates && addgroup -S goshorty && adduser -S -G goshorty goshorty
WORKDIR /app
COPY --from=build /out/goshorty /app/goshorty
USER goshorty
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:8080/ready || exit 1
ENTRYPOINT ["/app/goshorty"]
