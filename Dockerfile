FROM golang:1.22 AS build
WORKDIR /app
COPY . .
RUN go build -o /out/veloback ./cmd/api

FROM gcr.io/distroless/base-debian12
WORKDIR /srv
COPY --from=build /out/veloback /usr/local/bin/veloback
COPY data /srv/data
ENV VELOCLI_BACKEND_ADDR=0.0.0.0:9999
ENV VELOCLI_DATA_KEY_FILE=/srv/data/.key
EXPOSE 9999
ENTRYPOINT ["/usr/local/bin/veloback"]
