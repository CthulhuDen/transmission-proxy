FROM golang:alpine as build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . ./

RUN CGO_ENABLED=0 GOOS=linux go build -o /transmission-proxy ./cmd

FROM alpine

COPY --from=build /transmission-proxy /

EXPOSE 8080

CMD ["/transmission-proxy"]
