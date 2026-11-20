# syntax=docker/dockerfile:1

# ---- build ----
FROM golang:1.25-alpine AS build
WORKDIR /src

# 先只複製 go.mod/go.sum 下載依賴，原始碼變動時可重用這層快取。
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# ---- runtime ----
# distroless：只有執行檔與憑證，沒有 shell，體積小、攻擊面小。
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/server /server

ENV PORT=8080 \
    GIN_MODE=release \
    LOG_LEVEL=info \
    LOG_FORMAT=pretty
EXPOSE 8080
USER nonroot:nonroot

HEALTHCHECK --interval=10s --timeout=3s --start-period=3s --retries=3 \
    CMD ["/server", "-healthcheck"]

ENTRYPOINT ["/server"]
