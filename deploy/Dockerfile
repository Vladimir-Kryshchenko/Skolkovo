# Сборка сервиса «База Сколково» (multi-stage).
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/skolkovo ./cmd/skolkovo

# Runtime-образ с Chromium для headless-парсинга (каталог + fetch)
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates chromium libnss3 libnspr4 libatk1.0-0 libatk-bridge2.0-0 \
      libcups2 libdrm2 libgbm1 libpango-1.0-0 libgtk-3-0 libasound2 libxshmfence1 \
      curl wget \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /out/skolkovo /app/skolkovo
# Миграции БД нужны в рантайме: serve применяет их автоматически при старте.
COPY --from=build /src/deploy/migrations /app/migrations
EXPOSE 8080 8090
ENTRYPOINT ["/app/skolkovo"]
CMD ["serve"]
