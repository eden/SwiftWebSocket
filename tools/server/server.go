package main

import (
	"crypto/rand"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const rapidSize = 250 * 1024
const rapidFPS = 25

var port int
var crt, key string
var host string
var s string
var ports string
var _case string

func main() {

	flag.StringVar(&crt, "crt", "", "ssl cert file")
	flag.StringVar(&key, "key", "", "ssl key file")
	flag.StringVar(&host, "host", "localhost", "listening server host")
	flag.StringVar(&_case, "case", "", "choose a specialized case, (hang,rapid,pong)")
	flag.IntVar(&port, "port", 6789, "listening server port")
	flag.Parse()

	if crt != "" || key != "" {
		s = "s"
		if port != 443 {
			ports = fmt.Sprintf(":%d", port)
		}
	} else if port != 80 {
		ports = fmt.Sprintf(":%d", port)
	}
	http.HandleFunc("/client", client)
	http.HandleFunc("/echo", socket)
	log.Printf("Running server on %s:%d\n", host, port)
	switch _case {
	default:
		log.Fatalf("case: %s is unknown", _case)
	case "":
	case "hang":
		log.Printf("case: %s (long connection hanging)\n", _case)
	case "rapid":
		log.Printf("case: %s (rapid (250 fps) large (2048 bytes) random text messages)\n", _case)
	case "pong":
		log.Printf("case: pong (send pong events every 3 seconds)\n")
	}
	log.Printf("ws%s://%s%s/echo      (echo socket)\n", s, host, ports)
	log.Printf("http%s://%s%s/client  (javascript test client)\n", s, host, ports)
	var err error
	if crt != "" || key != "" {
		err = http.ListenAndServeTLS(fmt.Sprintf(":%d", port), crt, key, nil)
	} else {
		err = http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
	}
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

func socket(w http.ResponseWriter, r *http.Request) {
	log.Print("connection established")
	if _case == "hang" {
		hang := time.Minute
		log.Printf("hanging for %s\n", hang.String())
		time.Sleep(hang)
	}
	ws, err := websocket.Upgrade(w, r, nil, 1024, 1024)
	if err != nil {
		log.Print(err)
		return
	}
	defer func() {
		ws.Close()
		log.Print("connection closed")
	}()
	var mu sync.Mutex
	if _case == "rapid" {
		go func() {
			defer func() {
				ws.Close()
			}()
			msg := make([]byte, rapidSize)
			b := make([]byte, 2048)
			for {
				time.Sleep(time.Second / rapidFPS)
				i := 0
			outer:
				for {
					rand.Read(b)
					for _, c := range b {
						if i == len(msg) {
							break outer
						}
						msg[i] = (c % (126 - 32)) + 32 // ascii #32-126
						i++
					}
				}
				copy(msg, []byte(time.Now().String()+"\n"))
				mu.Lock()
				if err := ws.WriteMessage(websocket.TextMessage, msg); err != nil {
					mu.Unlock()
					return
				}
				mu.Unlock()
			}
		}()
	}
	if _case == "pong" {
		go func() {
			defer ws.Close()
			i := 0
			for range time.NewTicker(time.Second).C {
				mu.Lock()
				err := ws.WriteControl(websocket.PongMessage,
					[]byte(fmt.Sprintf("PONG:%d", i)), time.Now().Add(time.Second))
				mu.Unlock()
				if err != nil {
					return
				}
				i++
			}
		}()
	}
	for {
		msgt, msg, err := ws.ReadMessage()
		if err != nil {
			log.Print(err)
			return
		}
		log.Print("rcvd: '" + string(msg) + "'")
		time.Sleep(time.Second / 10)
		mu.Lock()
		ws.WriteMessage(msgt, msg)
		mu.Unlock()
	}
}

func client(w http.ResponseWriter, r *http.Request) {
	log.Print("client request")
	w.Header().Set("Content-Type", "text/html")
	if _case != "" {
		fmt.Fprintf(w, "<div style='color:green'>["+_case+"]</div>")
	}

	io.WriteString(w, `
		<pre id="out"></pre>
		<script>
		var console={log:function(s){document.getElementById("out").innerHTML+=s+"\n";}};
		var messageNum = 0;
		var ws = new WebSocket("`+fmt.Sprintf("ws%s://%s%s/echo", s, host, ports)+`")
        ws.onerror = function(ev){
            console.log("error " + ev)
        }
        ws.onclose = function(){
            console.log("close")
        }
	`)
	if _case == "rapid" {
		io.WriteString(w, `
        ws.onopen = function(){
        	console.log("opened")
        }
        ws.onmessage = function(msg){
        	document.getElementById("out").innerHTML = "recv: [" + msg.data.length + " bytes] " + msg.data.slice(0, msg.data.indexOf('\n')) + "\n"
        }
		`)
	} else {
		io.WriteString(w, `
		function send(){
			messageNum++;
            var msg = messageNum + ": " + new Date()
            console.log("send: " + msg)
            ws.send(msg)
        }
        ws.onopen = function(){
        	console.log("opened")
            send()
        }
        ws.onmessage = function(msg){
            console.log("recv: " + msg.data)
            if (messageNum == 10) {
                ws.close()
            } else {
                send()
            }
        }
		`)
	}
	io.WriteString(w, `</script>`)

}
