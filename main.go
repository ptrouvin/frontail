package main

import (
	"flag"
	"html/template"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vharitonsky/iniflags"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	// Time allowed to write the file to the client.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the client.
	//pongWait = 60 * time.Second
	pongWait = 0

	// Send pings to client with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Poll file for changes with this period.
	filePeriod = 1 * time.Second
)

var (
	portPtr     = flag.Int("port", 8080, "port number as an int")
	loglevelPtr = flag.String("loglevel", "info", "Define the loglevel: debug,info,warning,error")
	skipRePtr   = flag.String("skip", "", "Define the regexp of characters to skip. Like '^[^{]*' to skip any leading chars before the first '{'.")
	homeTempl   = template.Must(template.New("").Parse(homeHTML))
	filename    = flag.String("filename", "", "Filename to publish")
	grepRePtr   = flag.String("grep", ".*", "Define the regexp to select lines to push")
	forceTLSPtr = flag.Bool("force-tls", false, "Force wss protocol whatever HTTP or HTTPS (usefull when behind a reverse-proxy)")
	upgrader    = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 4096,
	}
	filePosPerIP map[string]int64
	skipRe       *regexp.Regexp
	grepRe       *regexp.Regexp

	filePosPerIP_lock = sync.Mutex{}
)

func getFilePos(ip string) int64 {
	if lastPos, ok := filePosPerIP[ip]; ok {
		log.Debug().
			Str("ip", ip).
			Int64("lastPos", lastPos).
			Msg("getFilePos")
		return lastPos
	}
	log.Debug().
		Str("ip", ip).
		Int64("lastPos", 0).
		Msg("getFilePos")
	//filePosPerIP[ip]=0
	return 0
}

func setFilePos(ip string, lastPos int64) {
	log.Debug().
		Str("ip", ip).
		Int64("lastPos", lastPos).
		Int("filePosPerIP.length", len(filePosPerIP)).
		Msg("setFilePos")
	filePosPerIP_lock.Lock()
	filePosPerIP[ip] = lastPos
	filePosPerIP_lock.Unlock()
}

func readFileIfModified(lastMod time.Time, lastPos int64) ([]byte, time.Time, int64, error) {
	log.Debug().
		Int64("lastMod", lastMod.Unix()).
		Int64("lastPos", lastPos).
		Msg("Called")
	fi, err := os.Stat(*filename)
	if err != nil {
		log.Error().
			Err(err).
			Str("filename", *filename).
			Msg("Stat ERROR")
		return nil, lastMod, lastPos, err
	}
	if !fi.ModTime().After(lastMod) {
		log.Debug().
			Int64("file-modtime", fi.ModTime().Unix()).
			Msg("lastMod>ModTime")
		return nil, lastMod, lastPos, nil
	}
	f, err := os.Open(*filename)
	if err != nil {
		log.Error().
			Err(err).
			Str("filename", *filename).
			Msg("Open ERROR")
		return nil, lastMod, lastPos, nil
	}
	size2read := fi.Size() - lastPos
	if size2read <= 0 {
		// file rotation detected
		lastPos = 0
		size2read = fi.Size()
		log.Debug().
			Int64("fileSize", fi.Size()).
			Int64("lastPos", lastPos).
			Int64("size2read", size2read).
			Msg("file rotation: size2read<0")
	}
	p := []byte("")
	if size2read > 0 {
		lastPos, err = f.Seek(lastPos, 0)
		if err != nil {
			log.Error().
				Err(err).
				Int64("lastPos", lastPos).
				Msg("Seek Error")
			lastPos, err = f.Seek(0, 0)
		}
		log.Debug().
			Int64("fileSize", fi.Size()).
			Int64("curPos", lastPos).
			Msg("file Current position")
		p = make([]byte, size2read)
		count, err := f.Read(p)
		if err != nil {
			log.Error().
				Err(err).
				Int64("size2read", size2read).
				Msg("Read ERROR")
			return nil, fi.ModTime(), lastPos, err
		}
		if skipRe != nil {
			// a skip regexp was provided
			p1 := p
			p = skipRe.ReplaceAll(p1, []byte(""))
		}
		log.Debug().
			Int64("fileSize", fi.Size()).
			Int64("lastPos", lastPos).
			Int64("size2read", size2read).
			Int("byteReadCount", count).
			Int("data_length", len(p)).
			Msg("File READ")
	}
	curPos, err := f.Seek(0, 1)
	if err != nil {
		log.Error().
			Err(err).
			Msg("get last file pos Error")
		return nil, fi.ModTime(), 0, err
	}
	f.Close()
	log.Debug().
		Int64("ret_lastMod", fi.ModTime().Unix()).
		Int64("ret_lastPos", curPos).
		Int64("byte_count", size2read).
		Int("Data_length", len(string(p))).
		Msg("Returned data")
	return p, fi.ModTime(), curPos, nil
}

func reader(ws *websocket.Conn, wsIsConnected *int) {
	defer ws.Close()
	ws.SetReadLimit(512)
	if pongWait > 0 {
		ws.SetReadDeadline(time.Now().Add(pongWait))
	} else {
		ws.SetReadDeadline(time.Time{})
	}
	ws.SetPongHandler(func(string) error { ws.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		mtype, msg, err := ws.ReadMessage()
		if err != nil {
			log.Error().
				Err(err).
				Msg("reader.ReadMessage ERROR")
			*wsIsConnected = 0
			break
		}
		log.Info().
			Int("MessageType", mtype).
			Str("Message", string(msg)).
			Msg("reader.ReadMessage")
	}
}

func writer(ws *websocket.Conn, lastMod time.Time, oldLastPos int64, ip string, wsIsConnected *int) {
	lastPos := oldLastPos
	//pingTicker := time.NewTicker(pingPeriod)
	fileTicker := time.NewTicker(filePeriod)
	defer func() {
		//pingTicker.Stop()
		fileTicker.Stop()
		ws.Close()
	}()
	for *wsIsConnected == 1 {
		select {
		case <-fileTicker.C:
			var p []byte

			p, lastMod, lastPos, _ = readFileIfModified(lastMod, lastPos)

			if p != nil && len(p) > 0 {
				for _, line := range strings.Split(strings.TrimSuffix(string(p), "\n"), "\n") {
					if grepRe.Match([]byte(line)) {
						log.Debug().Msg("DBG>line sent " + line)
						ws.SetWriteDeadline(time.Now().Add(writeWait))
						if err := ws.WriteMessage(websocket.TextMessage, []byte(line+"\n")); err != nil {
							log.Error().
								Str("ip", ip).
								Msg("Closing connection, error on websocket")
							*wsIsConnected = 0
							break
						}
					} else {
						log.Debug().Msg("DBG>line dropped " + line)
					}
				}
			} else {
				time.Sleep(10 * time.Second)
			}
			setFilePos(ip, lastPos)

			/* case <-pingTicker.C:
			ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := ws.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
				*wsIsConnected = 0
				break
			} */
		}
	}
	log.Debug().Msg("Writer.Closing ws")
}

func logClientIP(r *http.Request) string {
	IPAddress := r.Header.Get("X-Real-Ip")
	if IPAddress == "" {
		IPAddress = r.Header.Get("X-Forwarded-For")
	}
	if IPAddress == "" {
		IPAddress = r.RemoteAddr
	}
	ip, port, err := net.SplitHostPort(IPAddress)
	if err != nil {
		log.Error().
			Err(err).
			Msgf("SplitHostPort: %q is not IP:port", IPAddress)
		return IPAddress
	}

	userIP := net.ParseIP(ip)
	if userIP == nil {
		log.Error().
			Msgf("userip: %q is not IP", IPAddress)
		return IPAddress
	}
	userIPstr := userIP.String()
	log.Info().
		Str("ip", userIPstr).
		Str("port", port).
		Msgf("Connected from %q:%q", userIPstr, port)
	return ip
}

func serveWs(w http.ResponseWriter, r *http.Request) {
	var ip = logClientIP(r)

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		if _, ok := err.(websocket.HandshakeError); !ok {
			log.Error().
				Err(err).
				Msg("Upgrade ERROR")
		}
		return
	}

	var wsIsConnected int = 1

	var lastMod time.Time
	if n, err := strconv.ParseInt(r.FormValue("lastMod"), 10, 64); err == nil {
		lastMod = time.Unix(n, 0)
	}

	var lastPos int64
	if n, err := strconv.ParseInt(r.FormValue("lastPos"), 10, 64); err == nil {
		lastPos = n
	} else {
		lastPos = getFilePos(ip)
	}

	//writer(ws, lastMod, lastPos, ip)
	go writer(ws, lastMod, lastPos, ip, &wsIsConnected)
	go reader(ws, &wsIsConnected)
}

func serveHome(w http.ResponseWriter, r *http.Request) {
	var ip = logClientIP(r)

	// if r.URL.Path != "/" {
	// 	http.Error(w, "Not found", http.StatusNotFound)
	// 	return
	// }
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var lastPos int64
	if n, err := strconv.ParseInt(r.FormValue("lastPos"), 10, 64); err == nil {
		lastPos = n
	} else {
		lastPos = getFilePos(ip)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	p, lastMod, lastPos, err := readFileIfModified(time.Time{}, lastPos)
	if err != nil {
		log.Error().
			Err(err).
			Msg("serverHome.readFileIfModified ERROR")
		p = []byte(err.Error())
		lastMod = time.Unix(0, 0)
		lastPos = getFilePos(ip)
	}

	// ws or wss
	method := "s"
	if !(*forceTLSPtr) && (r.URL.Scheme == "http" || len(r.URL.Scheme) == 0) {
		method = ""
	}

	// filtering strings
	p1 := ""
	for _, line := range strings.Split(strings.TrimSuffix(string(p), "\n"), "\n") {
		if grepRe.Match([]byte(line)) {
			log.Debug().Msg("DBG>line sent " + line)
			p1 += line + "\n"
		} else {
			log.Debug().Msg("DBG>line dropped " + line)
		}
	}

	log.Debug().
		Int("Data_length", len(string(p))).
		Int64("lastMod", lastMod.Unix()).
		Int64("lastPos", lastPos).
		Msg("Method: ws" + method + " serverHome.readFileIfModified")

	ws := strings.Trim(r.URL.Path, "/")
	if len(ws) > 0 && string(ws[len(ws)-1]) != "/" {
		ws += "/"
	}
	ws += "ws"

	var v = struct {
		Method   string
		Host     string
		Data     string
		LastMod  string
		LastPos  string
		Url      string
		Filename string
	}{
		method,
		r.Host,
		p1,
		strconv.FormatInt(lastMod.Unix(), 10),
		strconv.FormatInt(lastPos, 10),
		ws,
		*filename,
	}
	homeTempl.Execute(w, &v)
}

func serve(w http.ResponseWriter, r *http.Request) {
	if len(r.URL.Path) >= 3 && r.URL.Path[len(r.URL.Path)-3:] == "/ws" {
		serveWs(w, r)
	}
	serveHome(w, r)
}

func main() {
	iniflags.Parse()
	if len(*filename) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	switch *loglevelPtr {
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "info":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case "warning":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	default:
		flag.Usage()
		os.Exit(1)
	}

	if len(*skipRePtr) > 0 {
		skipRe = regexp.MustCompile(`(?m)` + *skipRePtr)
	}

	if len(*grepRePtr) > 0 {
		grepRe = regexp.MustCompile(`(?m)` + *grepRePtr)
	}

	var forceTLSstring = "FALSE"
	if *forceTLSPtr {
		forceTLSstring = "TRUE"
	}
	log.Info().
		Str("loglevel", *loglevelPtr).
		Msg("frontail started, monitoring file(" + *filename + ") SKIP=" + *skipRePtr + " GREP=" + *grepRePtr + " FORCE-TLS=" + forceTLSstring)

	filePosPerIP = make(map[string]int64)

	var router = http.HandlerFunc(serve)

	if err := http.ListenAndServe(":"+strconv.Itoa(*portPtr), router); err != nil {
		log.Fatal().
			Err(err).
			Msg("ListenAndServe ERROR")
	}
}

const homeHTML = `<!DOCTYPE html>
<html lang="en">
	<head>
		<meta charset="UTF-8">
		<meta name="description" content="Frontail: tail follow streaming file to the browser">
		<meta name="author" content="Krishna Modi <github.com/krish512>">
		<link rel="icon" href="data:;base64,iVBORw0KGgo=">
		<title>{{.Filename}}</title>
		<style>
			body {
				margin: 0;
				padding: 0;
			}
			header {
				display: flex;
				position: fixed;
				top: 0;
				padding: 20px 0;
				width: 100vw;
				background-color: black;
				color: white;
				font-size: 20px;
				font-family: sans-serif;
				justify-content: space-between;
			}
			#fileData {
				margin-top: 80px;
				padding: 0;
			}
			.log {
				padding: 0 10px;
				margin: 2px 0;
				white-space: pre-wrap;
				color: black;
				font-size: 1em;
				border: 0;
				cursor: default;
			}
			.selected {
				background-color: #ffb2b0;
			}
		</style>
    </head>
	<body>
		<header>
			<div style="padding: 0 20px;">File: {{.Filename}}</div>
			<div style="padding: 0 20px;"><input placeholder="filter" size="20" onkeyup="fmt(input,this.value)"></div>
		</header>
        <div id="fileData"></div>
		<script type="text/javascript">
			function fmt(input, filter="") {
				var lines = input.split("\n");
				data.innerHTML= "";
				var ll = lines.length;
				for(var i=0; i<ll; i++)
				{
					var regex = new RegExp( filter, 'ig' );
					if(filter == "" || filter == undefined || lines[i].match(regex)) {
						var elem = document.createElement('div');
						elem.className = "log";
						elem.addEventListener('click', function click() {
							if (this.className.indexOf('selected') === -1) {
							  this.className = 'log selected';
							} else {
							  this.className = 'log';
							}
						});
						elem.textContent = lines[i];
						data.appendChild(elem);
					}
				}
				window.scrollTo(0,document.body.scrollHeight);
			}

			var input = {{.Data}};
			var data = document.getElementById("fileData");
			fmt(input);

			var conn = new WebSocket("ws{{.Method}}://{{.Host}}/{{.Url}}?lastMod={{.LastMod}}&lastPos={{.LastPos}}");
			conn.onclose = function(evt) {
				data.textContent = 'Connection closed';
			}
			conn.onmessage = function(evt) {
				console.log('file updated');
				input = evt.data;
				fmt(input);
			}
        </script>
    </body>
</html>
`
