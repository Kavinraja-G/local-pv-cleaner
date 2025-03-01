FROM alpine:3.21.3

COPY local-pv-cleaner .

ENTRYPOINT ["./local-pv-cleaner"]