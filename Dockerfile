# syntax=docker/dockerfile:1
FROM golang:1.25-bookworm AS build
WORKDIR /src
# Local replace target (../telegram-gateway) is passed in as the named
# "telegram-gateway" build context (see docker-compose.yml additional_contexts).
COPY --from=telegram-gateway go.mod go.sum /telegram-gateway/
COPY --from=telegram-gateway pkg /telegram-gateway/pkg
COPY --from=telegram-gateway internal /telegram-gateway/internal
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/day-remind ./cmd/server

FROM debian:bookworm-slim
RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates \
  && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/day-remind /usr/local/bin/day-remind
EXPOSE 8090
ENTRYPOINT ["day-remind"]
