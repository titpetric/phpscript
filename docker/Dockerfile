FROM alpine:latest

ADD ./bin/phpscript-linux-amd64 /usr/local/bin/phpscript

RUN /usr/local/bin/phpscript version

WORKDIR /app
ENTRYPOINT ["phpscript"]
CMD ["server"]