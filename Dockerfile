FROM golang:1.13.1-buster

MAINTAINER Roma Erema

RUN export GOPATH="$HOME/go"

COPY build/wapi /etc/wapi/wapi
COPY remote_interaction/static /etc/wapi/remote_interaction/static

WORKDIR /etc/wapi

CMD ["/etc/wapi/wapi"]
