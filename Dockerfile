FROM golang:1.13-buster as build
WORKDIR /go/src/app
ADD *.go /go/src/app/
RUN go get -d -v ./...
RUN CGO_ENABLED=0 GOOS=linux go build -v -o /go/bin/app/tw2tg

FROM scratch
COPY config.yml config.yml
COPY --from=build /etc/ssl /etc/ssl
COPY --from=build /go/bin/app/tw2tg /tw2tg
CMD ["/tw2tg"]