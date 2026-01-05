## Home

A monorepo for my home services, running on a Raspberry PI Kubernetes
cluster.

The cluster is installed using [k3s](https://k3s.io/). To set this up:

- Install k3s on the primary node

  ```
  curl -sfL https://get.k3s.io | sh -s - server --disable servicelb --node-external-ip 192.168.1.2
  ```

- Install k3s on the camera node

  - Taint the node with "pi-camera=true:NoSchedule"

### `deploy`

The declarative Kubernetes configuration for deploying the applications.

### `home-root`

A root site for linking to the other services.

### `rpi-timelapse`

A web controlled timelapse camera, based around the [Raspberry PI Camera].

### `speed-tester`

A broadband speed test monitor, using [Speedtest by Ookla].

## Network

Router: 192.168.1.1

### Kubernetes

`k3s-manager-1` Node IP / SSH: 192.168.1.2

Kube API: 192.168.1.3

PiHole: 192.168.1.10

External ingress: 192.168.1.20

Internal ingress: 192.168.1.19

### Others

Home Assistant: 192.168.1.5

[raspberry pi camera]: https://www.raspberrypi.org/products/camera-module-v2/
[speedtest by ookla]: https://www.speedtest.net/
