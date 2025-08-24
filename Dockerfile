
# https://hub.docker.com/_/golang/tags
FROM golang:1.25.0 AS build
ENV CGO_ENABLED=0
WORKDIR /root/
RUN mkdir -p /root/tgfeed/
COPY *.go go.mod go.sum /root/tgfeed/
WORKDIR /root/tgfeed/
RUN go version
RUN go get -v
RUN ls -l -a
RUN go build -o tgfeed .
RUN ls -l -a


# https://hub.docker.com/_/alpine/tags
FROM alpine:3.22.1
RUN apk add --no-cache tzdata
RUN apk add --no-cache gcompat && ln -s -f -v ld-linux-x86-64.so.2 /lib/libresolv.so.2
RUN mkdir -p /opt/tgfeed/
COPY *.text /opt/tgfeed/
RUN ls -l -a /opt/tgfeed/
COPY --from=build /root/tgfeed/tgfeed /bin/tgfeed
WORKDIR /opt/tgfeed/
ENTRYPOINT ["/bin/tgfeed"]

