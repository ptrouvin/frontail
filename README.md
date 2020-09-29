# README #

Frontail: streaming file to the browser and follow tail

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