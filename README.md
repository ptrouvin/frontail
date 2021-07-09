# README # Frontail: streaming file to the browser and follow tail

- [README # Frontail: streaming file to the browser and follow tail](#readme--frontail-streaming-file-to-the-browser-and-follow-tail)
    - [What is this repository for?](#what-is-this-repository-for)
    - [Quick Start](#quick-start)
    - [Who do I talk to?](#who-do-i-talk-to)
  - [systemd service](#systemd-service)
- [DEBIAN package build](#debian-package-build)
- [Test with traefik](#test-with-traefik)
  

### What is this repository for?
Frontail is written in Go to provide a fast and easy way to stream contents of any file to the browser via an inbuilt web server and follow tail. This is inspired from [Frontail](https://github.com/mthenw/frontail]) by Maciej Winnicki, hence the name.

### Quick Start

* Get requirement
  - `go get github.com/gorilla/websocket`
  - `go get github.com/rs/zerolog`
* Build as `go build -o frontail main.go` or download a binary file from [Releases](https://github.com/krish512/frontail/releases) page
* Execute in shell `frontail -p 8080 /var/log/syslog`
* Visit [http://127.0.0.1:8080](http://127.0.0.1:8080)

### Who do I talk to? ###

* Repo owner or admin:
    `Krishna Modi <krish512@hotmail.com>`


## systemd service

Create the /lib/systemd/system/frontail.service file

```systemctl
[Unit]
Description=logstream using frontail

[Service]
Type=simple
ExecStart=/usr/local/bin/frontail -p 8081 /var/log/eos/logstream-for-tcl.log

[Install]
WantedBy=multi-user.target
```

Then
```shell
sudo systemctl start|stop|status|enable frontail
```

# DEBIAN package build

Look at [Packaging - getting started](https://wiki.debian.org/Packaging/Intro?action=show&redirect=IntroDebianPackaging)

* Build the source tar
```shell
tar czvf frontail_1.0.0.orig.tar.gz Dockerfile main.go README.md Makefile frontail.systemd frontail.1
```

```shell
cd build
tar xzvf ../frontail_1.0.0.orig.tar.gz
```

* maintain the changelog file

* build the package
```shell
debuild -us -uc
cd ..
```

* the location of the .deb package
Into the main directory:


# Test with traefik
* [traefik documentation](https://doc.traefik.io/traefik/providers/docker/)
* [traefik.yaml is located at](file:///home/pascal/Progs/oasis/logstream/frontail/traefik/traefik.yml)
* traefik container
```shell
docker run -ti --rm --name traefik -v $PWD:/etc/traefik -p 80:80 -p 443:443 -p 18080:8080 -v /var/run/docker.sock:/var/run/docker.sock traefik
```
* [traefik dashboard](http://127.0.0.1:18080/dashboard/#/http/routers/frontail@docker)

* rebuild the frontail container:
```shell
docker build -t frontail .
```

* launch frontail container
```shell
docker run -ti --name frontail --rm -v $PWD/test.data:/var/log/eos/logstream-for-tcl.log -l traefik.enable=true -v $PWD/config.ini:/etc/frontail/config.ini frontail
```

* [test with curl](https://frontail.docker.localhost)
```shell
curl -vk https://frontail.docker.localhost
```