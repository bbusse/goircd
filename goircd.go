/*
goircd -- minimalistic simple Internet Relay Chat (IRC) server
Copyright (C) 2014-2016 Sergey Matveev <stargrave@stargrave.org>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

package main

import (
	"crypto/tls"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"time"

	healthchecking "github.com/heptiolabs/healthcheck"
	"github.com/namsral/flag"

	proxyproto "github.com/Freeaqingme/go-proxyproto"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	PROXY_TIMEOUT   = 5
)

var (
	version      string
	hostname     = flag.String("hostname", "localhost", "Hostname")
	bind         = flag.String("bind", ":6667", "Address to bind to")
	motd         = flag.String("motd", "", "Path to MOTD file")
	logdir       = flag.String("logdir", "", "Absolute path to directory for logs")
	statedir     = flag.String("statedir", "", "Absolute path to directory for states")
	passwords    = flag.String("passwords", "", "Optional path to passwords file")
	tlsBind      = flag.String("tlsbind", "", "TLS address to bind to")
	tlsPEM       = flag.String("tlspem", "", "Path to TLS certificat+key PEM file")
	tlsKEY       = flag.String("tlskey", "", "Path to TLS key PEM as seperate file")
	tlsonly      = flag.Bool("tlsonly", false, "Disable listening on non tls-port")
	proxyTimeout = flag.Uint("proxytimeout", PROXY_TIMEOUT, "Timeout when using proxy protocol")
	metrics      = flag.Bool("metrics", false, "Enable metrics export")
	verbose      = flag.Bool("v", false, "Enable verbose logging.")
	healtcheck   = flag.Bool("healthcheck", false, "Enable healthcheck endpoint.")
	healtbind    = flag.String("healthbind", "[::]:8086", "Healthcheck bind address and port.")

	clients_tls_total = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "clients_tls_connected_total",
			Help: "Number of connected clients during the lifetime of the server.",
		},
	)

	clients_irc_total = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "clients_irc_connected_total",
			Help: "Number of connected irc clients during the lifetime of the server.",
		},
	)

	clients_irc_rooms_total = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "clients_irc_rooms_connected_total",
			Help: "Number of clients joined to rooms during the lifetime of the server.",
		},
		[]string{"room"},
	)

	clients_connected = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "clients_connected",
			Help: "Number of connected clients.",
		},
	)
)

func listenerLoop(sock net.Listener, events chan ClientEvent) {
	for {
		conn, err := sock.Accept()
		if err != nil {
			log.Println("Error during accepting connection", err)
			continue
		}
		client := NewClient(conn)
		clients_tls_total.Inc()
		go client.Processor(events)
	}
}

func Run() {
	events := make(chan ClientEvent)
	log.SetFlags(log.Ldate | log.Lmicroseconds | log.Lshortfile)

	if *logdir == "" {
		// Dummy logger
		go func() {
			for _ = range logSink {
			}
		}()
	} else {
		if !path.IsAbs(*logdir) {
			log.Fatalln("Need absolute path for logdir")
		}
		go Logger(*logdir, logSink)
		log.Println(*logdir, "logger initialized")
	}

	log.Println("goircd " + version + " is starting")
	if *statedir == "" {
		// Dummy statekeeper
		go func() {
			for _ = range stateSink {
			}
		}()
	} else {
		if !path.IsAbs(*statedir) {
			log.Fatalln("Need absolute path for statedir")
		}
		states, err := filepath.Glob(path.Join(*statedir, "#*"))
		if err != nil {
			log.Fatalln("Can not read statedir", err)
		}
		for _, state := range states {
			buf, err := ioutil.ReadFile(state)
			if err != nil {
				log.Fatalf("Can not read state %s: %v", state, err)
			}
			room, _ := RoomRegister(path.Base(state))
			contents := strings.Split(string(buf), "\n")
			if len(contents) < 2 {
				log.Printf("State corrupted for %s: %q", *room.name, contents)
			} else {
				room.topic = &contents[0]
				room.key = &contents[1]
				log.Println("Loaded state for room", *room.name)
			}
		}
		go StateKeeper(*statedir, stateSink)
		log.Println(*statedir, "statekeeper initialized")
	}

	proxyTimeout := time.Duration(uint(*proxyTimeout)) * time.Second

	if *bind != "" && !*tlsonly {
		listener, err := net.Listen("tcp", *bind)
		if err != nil {
			log.Fatalf("Can not listen on %s: %v", *bind, err)
		}
		// Add PROXY-Protocol support
		listener = &proxyproto.Listener{Listener: listener, ProxyHeaderTimeout: proxyTimeout}

		log.Println("Raw listening on", *bind)
		go listenerLoop(listener, events)
	}

	if *tlsBind != "" {
		if *tlsKEY == "" {
			tlsKEY = tlsPEM
		}

		cert, err := tls.LoadX509KeyPair(*tlsPEM, *tlsKEY)

		if err != nil {
			log.Fatalf("Could not load Certificate and TLS keys from %s: %s", *tlsPEM, *tlsKEY, err)
		}
		config := tls.Config{Certificates: []tls.Certificate{cert}}

		listenerTLS, err := net.Listen("tcp", *tlsBind)
		if err != nil {
			log.Fatalf("Can not listen on %s: %v", *tlsBind, err)
		}
		log.Println("TLS listening on", *tlsBind)

		// Add PROXY-Protocol support

		listenerTLS = &proxyproto.Listener{Listener: listenerTLS, ProxyHeaderTimeout: proxyTimeout}

		listenerTLS = tls.NewListener(listenerTLS, &config)

		go listenerLoop(listenerTLS, events)
	}

	// Create endpoint for prometheus metrics export
	if *metrics {
		go prom_export()
	}
	if *healtcheck {
		go health_endpoint()
	}

	Processor(events, make(chan struct{}))
}

func health_endpoint() {
	var (
		health_bind = "0.0.0.0:8086"
	)
	health := healthchecking.NewHandler()
	health.AddLivenessCheck("goroutine-threshold", healthchecking.GoroutineCountCheck(100))
	log.Printf("Healthcheck listening on http://%s", health_bind)
	http.ListenAndServe(health_bind, health)
}

func prom_export() {
	prometheus.MustRegister(clients_tls_total)
	prometheus.MustRegister(clients_irc_total)
	prometheus.MustRegister(clients_irc_rooms_total)
	prometheus.MustRegister(clients_connected)

	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func main() {
	flag.Parse()
	Run()
}
