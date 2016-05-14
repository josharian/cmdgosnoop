package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

var (
	flagVerbose = flag.Bool("v", false, "be verbose")
	flagDaemon  = flag.Bool("d", false, "daemonize; do not call directly")
)

func main() {
	flag.Parse()
	if *flagDaemon {
		daemonize()
		return
	}
	attempts := 0
	for {
		_, err := http.Get("http://localhost:10808/ping")
		if err == nil {
			break
		}
		cmd := exec.Command(os.Args[0], "-d")
		err = cmd.Start()
		check(err)
		err = cmd.Process.Release()
		check(err)
		attempts++
		if attempts > 10 {
			log.Fatal("failed to start daemon")
		}
	}
	tool := filepath.Base(flag.Arg(0))
	args := flag.Args()[1:]
	pkg := os.Getenv("TOOLEXEC_PKG_PATH")
	if pkg == "" {
		fmt.Printf("%v\nmissing TOOLEXEC_PKG_PATH; are you using an appropriately hacked version of cmd/go?\n", flag.Args())
		os.Exit(1)
	}
	var b [10]byte
	_, err := io.ReadFull(rand.Reader, b[:])
	check(err)
	id := fmt.Sprintf("%x", b[:])
	e := event{
		ID:   id,
		Kind: "start",
		When: time.Now(),
		Tool: tool,
		Pkg:  pkg,
	}
	e.post()
	if *flagVerbose {
		fmt.Println("Running", flag.Arg(0), args)
	}
	cmd := exec.Command(flag.Arg(0), args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.Exit(1)
	}
	e.Kind = "stop"
	e.When = time.Now()
	e.post()
}

func (e event) post() {
	body, err := json.Marshal(e)
	check(err)
	_, err = http.Post("http://localhost:10808/event", "encoding/json", bytes.NewReader(body))
	check(err)
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

type event struct {
	ID   string    // unique id
	Kind string    // start, stop
	When time.Time // as reported, to avoid network latency goop
	Tool string    // what tool was invoked
	Pkg  string    // package path
}

type eventByTime []event

func (x eventByTime) Len() int           { return len(x) }
func (x eventByTime) Swap(i, j int)      { x[i], x[j] = x[j], x[i] }
func (x eventByTime) Less(i, j int) bool { return x[i].When.Before(x[j].When) }

type server struct {
	evc chan struct{}

	mu   sync.Mutex
	all  []event
	live map[string]event
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.evc <- struct{}{}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch r.URL.Path {
	case "/status":
		fmt.Fprintf(w, "server: %#v\n", s)
		return
	case "/die":
		os.Exit(1)
	case "/chart":
		sort.Sort(eventByTime(s.all))
		live := make(map[string]event)
		for _, e := range s.all {
			switch e.Kind {
			case "start":
				live[e.ID] = e
			case "stop":
				delete(live, e.ID)
			}
			var cmds []string
			for _, ee := range live {
				cmds = append(cmds, "'"+ee.Tool+" "+ee.Pkg+"'")
			}
			sort.Strings(cmds)
			fmt.Fprintln(w, e.When.Format("15:04:05.00"), len(live), cmds)
		}
		return
	case "/event":
	default:
		http.Error(w, "bad path "+r.URL.Path, http.StatusBadRequest)
		return
	}
	dec := json.NewDecoder(r.Body)
	defer r.Body.Close()
	var e event
	err := dec.Decode(&e)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.all = append(s.all, e)
	switch e.Kind {
	case "start":
		s.live[e.ID] = e
	case "stop":
		delete(s.live, e.ID)
	default:
		http.Error(w, "bad event kind "+e.Kind, http.StatusBadRequest)
		return
	}
	fmt.Println("got event", e)
}

func daemonize() {
	fmt.Println("starting daemon")
	s := server{
		evc:  make(chan struct{}),
		live: make(map[string]event),
	}
	http.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {})
	http.Handle("/event", &s)
	http.Handle("/status", &s)
	http.Handle("/die", &s)
	http.Handle("/chart", &s)
	go func() {
		log.Fatal(http.ListenAndServe(":10808", nil))
	}()
	for {
		select {
		case <-s.evc:
		case <-time.After(time.Second * 15):
			fmt.Println("timed out")
			return
		}
	}
}
