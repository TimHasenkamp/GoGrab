# syntax=docker/dockerfile:1.7

# ---------- 1) Build the SvelteKit frontend ----------
FROM node:26-alpine AS web-builder
WORKDIR /web
COPY web/package.json web/package-lock.json* ./
RUN npm ci --no-audit --no-fund --prefer-offline || npm install --no-audit --no-fund
COPY web/ ./
RUN npm run build

# ---------- 2) Build the Go binary (cross-compiled per TARGETARCH) ----------
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS go-builder
WORKDIR /src
ARG TARGETARCH
ENV CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY web/web.go ./web/web.go
# Pull in the freshly built frontend so go:embed picks it up.
COPY --from=web-builder /web/build ./web/build
RUN go build -trimpath -ldflags="-s -w" -o /out/gograb ./cmd/server

# ---------- 3) Distroless runtime ----------
FROM gcr.io/distroless/static-debian12:nonroot AS runtime
WORKDIR /app
COPY --from=go-builder /out/gograb /app/gograb
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/gograb"]
