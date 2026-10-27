FROM golang:1.27.3-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /roomflow ./cmd/api

FROM alpine:3.22
RUN addgroup -S app && adduser -S -G app app
USER app
COPY --from=build /roomflow /roomflow
EXPOSE 8080
CMD ["/roomflow"]
