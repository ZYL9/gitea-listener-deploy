FROM golang:alpine3.19 as BUILD
ENV GOOS=linux
ENV GOARCH=amd64
WORKDIR /usr/src/app
COPY main.go go.mod ./
RUN go build .

FROM alpinelinux/docker-cli:latest as PROD
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.tuna.tsinghua.edu.cn/g' /etc/apk/repositories &&\
    apk update &&\
    apk add git
COPY --from=BUILD /usr/src/app/listener /opt/
COPY start.sh /opt/
RUN chmod +x /opt/listener &&\
    chmod +x /opt/start.sh
EXPOSE 3001
CMD /opt/listener