FROM golang:1.10 AS goircd-builder
ARG  PACKAGE=github.com/bbusse/goircd
ENV  PACKAGE=$PACKAGE

WORKDIR /go/src/$PACKAGE/

ADD  . /go/src/$PACKAGE/

RUN  export CGO_ENABLED=0 \
 &&  go get $PACKAGE \
 &&  make -f GNUmakefile goircd \
 &&  mv goircd /go/bin/goircd

FROM alpine AS goircd
COPY --from=goircd-builder /go/bin/goircd /bin/goircd
ENTRYPOINT ["sh","-c"]
CMD ["exec goircd"]