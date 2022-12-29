FROM golang:1.18 AS builder

# ENV GOPROXY      https://goproxy.io

RUN mkdir /app
ADD . /app/
WORKDIR /app
RUN sed -i 's@localhost:389@openldap:389@g' /app/config.yml \
    && sed -i 's@host: localhost@host: mysql@g'  /app/config.yml && go build -o ldap-admin-server .

FROM centos:centos7
RUN mkdir /app /app/key /app/conf
WORKDIR /app
COPY --from=builder /app/ldap-admin-server .
COPY --from=builder *.pem /app/key
COPY --from=builder config.yml /app/conf
COPY --from=builder rbac_model.conf /app/conf
RUN chmod +x wait ldap-admin-server

CMD ./wait && ./ldap-admin-server
