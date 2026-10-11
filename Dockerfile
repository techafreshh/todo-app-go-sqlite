FROM golang:1.26-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server .

FROM gcr.io/distroless/static-debian12

WORKDIR /app

COPY --from=builder /out/server /app/server
COPY --from=builder /src/static /app/static

EXPOSE 8080
VOLUME /data

ENV PORT=8080 \
    DB_PATH=/data/todos.db

ENTRYPOINT ["/app/server"]
