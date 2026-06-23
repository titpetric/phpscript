FROM alpine:latest

ADD ./bin/phpscript /usr/local/bin
RUN /usr/local/bin/phpscript version

WORKDIR /app
ENTRYPOINT ["phpscript"]
CMD ["server"]