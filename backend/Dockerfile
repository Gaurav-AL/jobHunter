FROM golang:1.25-alpine AS build

WORKDIR /app

RUN apk add --no-cache ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/jobhunter-api ./cmd

FROM alpine:3.22

WORKDIR /app

RUN apk add --no-cache ca-certificates poppler-utils && adduser -D -H appuser

COPY --from=build /out/jobhunter-api /usr/local/bin/jobhunter-api

RUN mkdir -p /app/uploads && chown -R appuser:appuser /app
USER appuser

EXPOSE 8080

CMD ["jobhunter-api"]
