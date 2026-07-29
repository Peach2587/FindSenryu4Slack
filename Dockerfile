# Build stage
FROM golang:1.26-trixie AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# No CGO in the MVP (kagome + slack-go are pure Go), so we produce a fully
# static binary that runs on distroless/static.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bot .

# Runtime stage
FROM gcr.io/distroless/static-debian13:nonroot

WORKDIR /app
COPY --from=builder /build/bot /app/bot

ENTRYPOINT ["/app/bot"]
