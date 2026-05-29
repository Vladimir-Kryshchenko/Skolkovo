# Сборка сервиса «База Сколково» (multi-stage).
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/skolkovo ./cmd/skolkovo

FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=build /out/skolkovo /app/skolkovo
# Том для первоисточников и отчётов монтируется снаружи.
EXPOSE 8080 8090
ENTRYPOINT ["/app/skolkovo"]
CMD ["serve"]
