## Home

A monorepo for my home services, running on a Raspberry PI Kubernetes
cluster.

The cluster is installed using [k3s](https://k3s.io/). To set this up:
 - Install k3s on the primary node
 - Install k3s on the camera node
   - Taint the node with "pi-camera=true:NoSchedule"
 - Install cert manager onto cluster
 - Add port forwarding rules to cluster

### `deploy`

The declarative Kubernetes configuration for deploying the applications.

### `home-root`

A root site for linking to the other services.

### `rpi-timelapse`

A web controlled timelapse camera, based around the [Raspberry PI Camera].

### `speed-tester`

A broadband speed test monitor, using [Speedtest by Ookla]. 

[Raspberry PI Camera]: https://www.raspberrypi.org/products/camera-module-v2/
[Speedtest by Ookla]: https://www.speedtest.net/