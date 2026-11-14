FROM golang:1.23-alpine AS build
WORKDIR /src
COPY . .
RUN go mod tidy && CGO_ENABLED=0 go build -o /out/dns ./cmd/dns && CGO_ENABLED=0 go build -o /out/api ./cmd/api

FROM alpine:3.20 AS dns
RUN apk add --no-cache ca-certificates && adduser -D app
COPY --from=build /out/dns /dns
# Порты 53/853 < 1024: в контейнере работаем от root; на хосте можно ограничить capabilities.
CMD ["/dns"]

FROM alpine:3.20 AS api
RUN apk add --no-cache ca-certificates && adduser -D app
COPY --from=build /out/api /api
USER app
CMD ["/api"]
