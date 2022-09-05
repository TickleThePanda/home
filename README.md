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

- Add port forwarding rules to cluster:
  - 22 -> 192.168.1.2:22
  - 8443 -> 192.168.1.2:8443
  - 80 -> 192.168.1.10:80
  - 443 -> 192.168.1.10:443

### `deploy`

The declarative Kubernetes configuration for deploying the applications.

### `home-root`

A root site for linking to the other services.

### `rpi-timelapse`

A web controlled timelapse camera, based around the [Raspberry PI Camera].

### `speed-tester`

A broadband speed test monitor, using [Speedtest by Ookla].

[raspberry pi camera]: https://www.raspberrypi.org/products/camera-module-v2/
[speedtest by ookla]: https://www.speedtest.net/
