FROM golang:1.19-alpine as build

RUN adduser -D build
USER build
WORKDIR /home/build

COPY . .
RUN go build

FROM alpine

ARG USER=kubecfg
RUN adduser -D $USER
USER $USER
WORKDIR /home/$USER

COPY --from=build /home/build/kubecfg /usr/app/
ENTRYPOINT ["/usr/app/kubecfg"]
