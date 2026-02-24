FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /airadar ./cmd/airadar

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /airadar /usr/local/bin/airadar
VOLUME /data
ENV AIRADAR_DB_PATH=/data/airadar.db
EXPOSE 8080
ENTRYPOINT ["airadar"]
CMD ["run", "--port", "8080"]
