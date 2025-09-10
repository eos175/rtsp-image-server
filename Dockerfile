FROM ghcr.io/hybridgroup/opencv:4.12.0 AS build-stage

ENV GOPATH /go

RUN apt-get update && apt-get install -y git ca-certificates


WORKDIR /app

# Copiar el código fuente
COPY . .

# Compilar la app
RUN go build -o app .

FROM gcr.io/distroless/base-debian12 AS release-stage

WORKDIR /app

# # OpenCV 4.11.0 shared objects from build-stage
COPY --from=build-stage /usr/local/lib /usr/local/lib
COPY --from=build-stage /usr/local/lib/pkgconfig /usr/local/lib/pkgconfig
COPY --from=build-stage /usr/local/include /usr/local/include
COPY --from=build-stage /usr/local/lib/pkgconfig/opencv4.pc /usr/local/lib/pkgconfig/opencv4.pc
COPY --from=build-stage /usr/local/include/opencv4 /usr/local/include/opencv4
COPY --from=build-stage /usr/lib/x86_64-linux-gnu/ /usr/lib/x86_64-linux-gnu/
COPY --from=build-stage /lib/x86_64-linux-gnu/ /lib/x86_64-linux-gnu/
COPY --from=build-stage /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copiar el binario compilado
COPY --from=build-stage /app/app /app/app

ENV PKG_CONFIG_PATH /usr/local/lib/pkgconfig
ENV LD_LIBRARY_PATH /usr/local/lib
ENV CGO_CPPFLAGS -I/usr/local/include

EXPOSE 8080

ENTRYPOINT ["./app"]

