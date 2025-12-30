# build
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -o /out/watcher ./cmd/watcher

# run
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata \
  && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /out/watcher /app/watcher
COPY web/templates /app/web/templates
COPY web/static /app/web/static

ENV DB_PATH=/data/pulls.sqlite
ENV LISTEN_ADDR=:8080
VOLUME ["/data"]
EXPOSE 8080
ENTRYPOINT ["/app/watcher"]
