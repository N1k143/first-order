FROM golang:1.26.11-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /api ./cmd/api && CGO_ENABLED=0 go build -trimpath -o /migrate ./cmd/migrate
FROM alpine:3.22
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app
WORKDIR /app
COPY --from=build /api /migrate ./
COPY migrations ./migrations
COPY testdata/companies.csv ./testdata/companies.csv
USER app
ENV API_ADDR=0.0.0.0:18080
EXPOSE 18080
CMD ["/app/api"]
