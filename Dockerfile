FROM debian

RUN mkdir -p /etc/frontail /var/log/eos /app/ && adduser --disabled-password --no-create-home --gecos "" --home "/app" frontail
COPY frontail /app/
COPY config.ini /etc/frontail/

WORKDIR /app
EXPOSE 8081

VOLUME [ "/var/log/eos" ]

USER frontail

CMD ["/app/frontail","-config=/etc/frontail/config.ini"]