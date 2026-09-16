FROM golang:1.27-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
ENV CGO_ENABLED=0
RUN go build -o /mevius ./cmd/mevius

FROM alpine:3.19

RUN adduser -D -u 1001 mevius

COPY --from=build /mevius /usr/local/bin/mevius

RUN mkdir /data && chown mevius:mevius /data

VOLUME /data

USER mevius

EXPOSE 8080

ENTRYPOINT ["mevius"]