FROM  golang:1.9.2

WORKDIR /go/src/github.com/logzio/logzio-logging-plugin/

COPY . /go/src/github.com/logzio/logzio-logging-plugin/

RUN curl https://raw.githubusercontent.com/golang/dep/master/install.sh | sh

RUN cd /go/src/github.com/logzio/logzio-logging-plugin && go get -v

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /bin/logzio-logging-plugin .

FROM alpine:3.12
RUN apk --no-cache add ca-certificates
COPY --from=0 /bin/logzio-logging-plugin /bin/
WORKDIR /bin/
ENTRYPOINT ["/bin/logzio-logging-plugin"]