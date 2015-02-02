// Package netflixoss implements a simulation of a global large scale microservice architecture
// It creates and controls a collection of aws, netflixoss and netflix application microservices
// or reads in a network from a json file. It also logs the architecture (nodes and links) as it evolves
package netflixoss

import (
	"fmt"
	"github.com/adrianco/spigo/edda" // configuration logger
	"github.com/adrianco/spigo/elb"  // elastic load balancer
	"github.com/adrianco/spigo/gotocol"
	"github.com/adrianco/spigo/graphjson"
	"github.com/adrianco/spigo/karyon" // business logic microservice
	"github.com/adrianco/spigo/pirate" // random end user network
	"github.com/adrianco/spigo/zuul"   // API proxy microservice router
	"log"
	"math/rand"
	"time"
)

// Population count of netflixoss microservices to create
var Population int

// Run duration is set via command line flag to tell how long to let netflixoss run for
var RunSleep time.Duration

// Msglog toggles whether to log every message received to the console
var Msglog bool

// noodles channels mapped by microservice name connects netflixoss to everyone
var noodles map[string]chan gotocol.Message
var names []string
var listener chan gotocol.Message

// Reload the network from a file
func Reload(arch string) {
	pirate.Msglog = Msglog // pass on console message log flag if set
	elb.Msglog = Msglog
	zuul.Msglog = Msglog
	karyon.Msglog = Msglog
	listener = make(chan gotocol.Message) // listener for netflixoss
	log.Println("netflixoss reloading from " + arch + ".json")
	g := graphjson.ReadArch(arch)
	Population = 0 // just to make sure
	// count how many nodes there are
	for _, element := range g.Graph {
		if element.Node != "" {
			Population++
		}
	}
	// create the map of channels
	noodles = make(map[string]chan gotocol.Message, Population)
	// Start all the services
	for _, element := range g.Graph {
		if element.Node != "" && element.Service != "" {
			name := element.Node
			noodles[name] = make(chan gotocol.Message)
			// start the service and tell it it's name
			switch element.Service {
			case "pirate":
				go pirate.Start(noodles[name])
			case "elb":
				go elb.Start(noodles[name])
			case "zuul":
				go zuul.Start(noodles[name])
			case "karyon":
				go karyon.Start(noodles[name])
			default:
				log.Fatal("netflixoss: unknown service: " + element.Service)
			}
			noodles[name] <- gotocol.Message{gotocol.Hello, listener, name}
			if edda.Logchan != nil {
				// tell the service to report itself and new edges to the logger
				noodles[name] <- gotocol.Message{gotocol.Inform, edda.Logchan, ""}
			}
		}
	}
	// Make all the connections
	for _, element := range g.Graph {
		if element.Edge != "" && element.Source != "" && element.Target != "" {
			noodles[element.Source] <- gotocol.Message{gotocol.NameDrop, noodles[element.Target], element.Target}
			log.Println("Link " + element.Source + " > " + element.Target)
		}
	}
	// start the simulation chatting
	for name, noodle := range noodles {
		if name == "elb" {
			// tell each elb to start calling microservices every 0.1 to 1 secs
			delay := fmt.Sprintf("%dms", 100+rand.Intn(900))
			noodle <- gotocol.Message{gotocol.Chat, nil, delay}
		}
	}
	shutdown()
}

// Start netflixoss and create new microservices
func Start() {
	pirate.Msglog = Msglog // pass on console message log flag if set
	elb.Msglog = Msglog
	zuul.Msglog = Msglog
	karyon.Msglog = Msglog
	listener = make(chan gotocol.Message) // listener for netflixoss
	if Population < 1 {
		log.Fatal("netflixoss: can't create less than 1 microservice")
	} else {
		log.Printf("netflixoss: scaling to %v%%", Population)
	}
	// create map of channels and a name index to select randoml nodes from
	noodles = make(map[string]chan gotocol.Message, Population)
	names = make([]string, Population) // approximate size for indexable name list
	// we need an elb as a front end to spread request traffic around each endpoint
	// elb for api endpoint
	elbname := "elb-api"
	noodles[elbname] = make(chan gotocol.Message)
	go elb.Start(noodles[elbname])
	// setup the elb's name and logging, set chat rate after everything else is started
	noodles[elbname] <- gotocol.Message{gotocol.Hello, listener, elbname}
	if edda.Logchan != nil {
		// tell the pirate to report itself and new edges to the logger
		noodles[elbname] <- gotocol.Message{gotocol.Inform, edda.Logchan, ""}
	}
	// connect elb to it's initial dependencies
	// start zuul api proxies next
	zuulcount := 9 * Population / 100
	for i := 0; i < zuulcount; i++ {
		zuulname := fmt.Sprintf("zuul%v", i)
		noodles[zuulname] = make(chan gotocol.Message)
		go zuul.Start(noodles[zuulname])
		noodles[zuulname] <- gotocol.Message{gotocol.Hello, listener, zuulname}
		zone := fmt.Sprintf("zone zone%v", i%3)
		noodles[zuulname] <- gotocol.Message{gotocol.Put, nil, zone}
		if edda.Logchan != nil {
			// tell the microservice to report itself and new edges to the logger
			noodles[zuulname] <- gotocol.Message{gotocol.Inform, edda.Logchan, ""}
		}
		// hook all the zuul proxies up to the elb
		noodles[elbname] <- gotocol.Message{gotocol.NameDrop, noodles[zuulname], zuulname}
	}
	// start karyon business logic
	karyoncount := 27 * Population / 100
	for i := 0; i < karyoncount; i++ {
		karyonname := fmt.Sprintf("karyon%v", i)
		noodles[karyonname] = make(chan gotocol.Message)
		go karyon.Start(noodles[karyonname])
		noodles[karyonname] <- gotocol.Message{gotocol.Hello, listener, karyonname}
		zone := fmt.Sprintf("zone zone%v", i%3)
		noodles[karyonname] <- gotocol.Message{gotocol.Put, nil, zone}
		if edda.Logchan != nil {
			// tell the microservice to report itself and new edges to the logger
			noodles[karyonname] <- gotocol.Message{gotocol.Inform, edda.Logchan, ""}
		}
		// connect all the karyon in a zone to all zuul in that zone only
		for j := i % 3; j < zuulcount; j = j + 3 {
			zuul := fmt.Sprintf("zuul%v", j)
			noodles[zuul] <- gotocol.Message{gotocol.NameDrop, noodles[karyonname], karyonname}
		}
	}
	// tell this elb to start chatting with microservices every 0.1 secs
	delay := fmt.Sprintf("%dms", 100)
	log.Println("netflixoss: elb activity rate ", delay)
	noodles[elbname] <- gotocol.Message{gotocol.Chat, nil, delay}
	shutdown()
}

// Shutdown netflixoss and elb
func shutdown() {
	var msg gotocol.Message
	// wait until the delay has finished
	if RunSleep >= time.Millisecond {
		time.Sleep(RunSleep)
	}
	log.Println("netflixoss: Shutdown")
	for _, noodle := range noodles {
		gotocol.Message{gotocol.Goodbye, nil, "shutdown"}.GoSend(noodle)
	}
	for len(noodles) > 0 {
		msg = <-listener
		if Msglog {
			log.Printf("netflixoss: %v\n", msg)
		}
		switch msg.Imposition {
		case gotocol.Goodbye:
			delete(noodles, msg.Intention)
			if Msglog {
				log.Printf("netflixoss: netflixoss %v shutdown, population: %v    \n", msg.Intention, len(noodles))
			}
		}
	}
	if edda.Logchan != nil {
		close(edda.Logchan)
	}
	log.Println("netflixoss: Exit")
}
