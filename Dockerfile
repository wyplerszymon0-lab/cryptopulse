# ── build stage ──────────────────────────────────────────────────────────────
FROM golang:1.27-alpine AS builder

WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o predictor .

# ── runtime stage (scratch = minimal attack surface, ~5 MB image) ─────────────
FROM scratch
COPY --from=builder /app/predictor /predictor
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

ENTRYPOINT ["/predictor"]
CMD []
