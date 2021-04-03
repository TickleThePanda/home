# Version 3.13 on Raspberry Pi is not able to access network and the
# easiest workaround is to use 3.12.
# https://gitlab.alpinelinux.org/alpine/aports/-/issues/12091
ARG ALPINE_VERSION=3.12

# Build raspistill as part of the Raspberry PI userland binaries. We
# build use alpine to do this to ensure that references to mtrace and
# execinfo aren't included in the final binaries. mtrace and execinfo
# aren't easily available on alpine.
FROM alpine:${ALPINE_VERSION} AS camera-binaries

RUN mkdir /build
WORKDIR /build

RUN apk add --update --no-cache \
    cmake \
    gcc \
    g++ \
    linux-headers \
    make

ARG USERLAND_COMMIT=3fd8527eefd8790b4e8393458efc5f94eb21a615
RUN wget "https://github.com/raspberrypi/userland/archive/$USERLAND_COMMIT.zip" -O userland.zip && \
    unzip userland.zip && \
    rm userland.zip

RUN cd userland-${USERLAND_COMMIT} && \
    mkdir build && \
    cd build && \
    CXX=g++ cmake \ 
        -DSKIP_TAINTED_CHECK=true \
        -DCMAKE_BUILD_TYPE=armv71 \
        -DARM64=OFF \
        --target raspistill \
        ../ && \
    make -j4 --output-sync=recurse && \
    make install

# Build the timelapse binary.
FROM golang:alpine${ALPINE_VERSION} as go-build

RUN apk add --update --no-cache \
    g++

RUN mkdir /app
WORKDIR /app

COPY go.* ./
RUN go mod download

COPY src/ src/

RUN go vet src/*.go
RUN go build -o timelapse src/*.go


# Build the web application image
FROM alpine:${ALPINE_VERSION}

WORKDIR /app

RUN apk add --update --no-cache \
    libc6-compat \
    musl \
    psmisc

COPY --from=camera-binaries /opt/vc /opt/vc
ENV PATH="${PATH}:/opt/vc/bin"
ENV LD_LIBRARY_PATH="/opt/vc/lib"

COPY --from=go-build /app/timelapse /app

VOLUME /data

CMD ./timelapse
