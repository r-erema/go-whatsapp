FROM golang:1.13.1-buster

MAINTAINER Roma Erema

RUN export GOPATH="$HOME/go"


COPY build/wapi /etc/wapi/wapi

WORKDIR /etc/wapi

CMD ["/etc/wapi/wapi"]
