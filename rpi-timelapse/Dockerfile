FROM alpine:3.12 AS camera-binaries

RUN mkdir /build
WORKDIR /build

RUN apk add --update --no-cache \
    cmake \
    gcc \
    g++ \
    git \
    linux-headers \
    make

RUN git clone https://github.com/raspberrypi/userland.git . && \
    git checkout 3fd8527eefd8790b4e8393458efc5f94eb21a615 --quiet

RUN mkdir build && \
    cd build && \
    CXX=g++ cmake -DHAVE_MTRACE=false -DSKIP_TAINTED_CHECK=true -DCMAKE_BUILD_TYPE=armv71 -DARM64=OFF ../ && \
    make && \
    make install

FROM golang:alpine3.12

RUN apk add --update --no-cache \
    libc6-compat \
    musl \
    psmisc

COPY --from=camera-binaries /opt/vc /opt/vc
ENV PATH="${PATH}:/opt/vc/bin"
ENV LD_LIBRARY_PATH="/opt/vc/lib"

RUN mkdir /app
WORKDIR /app
COPY go.* ./
RUN go mod download

COPY src/ src/

RUN go vet *.go

VOLUME /data

CMD go run src/*.go

