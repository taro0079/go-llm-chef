# syntax=docker/dockerfile:1
FROM golang:1.27.3-bookworm AS build

WORKDIR /src

# go-sqlite3 requires CGO and the C compiler in the Go image.
ENV CGO_ENABLED=1
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
RUN go build -trimpath -ldflags="-s -w" -o /out/go-llm-chef .

FROM debian:bookworm-slim

# HTTPS certificates are needed for requests to the OpenAI API.
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 10001 chef \
    && useradd --uid 10001 --gid chef --no-create-home chef \
    && mkdir /data \
    && chown chef:chef /data

COPY --from=build /out/go-llm-chef /usr/local/bin/go-llm-chef

# The application stores ./app.db relative to its working directory.
WORKDIR /data
USER chef:chef
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/go-llm-chef"]
