package chatapi

import (
	"io"
	"log"
	"reflect"
	"strings"
	"sync"
)

//Room type represents a chat room
type Room struct {
	scope   []string
	name    string
	Msgch   chan string
	clients map[string]*client
	//signals the quitting of the chat room
	Quit chan struct{}
	*sync.RWMutex
}

//CreateRoom starts a new chat room with name rname
func CreateRoom(rname string) *Room {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	r := &Room{
		name:    rname,
		Msgch:   make(chan string),
		RWMutex: new(sync.RWMutex),
		clients: make(map[string]*client),
		Quit:    make(chan struct{}),
	}
	r.Run()
	return r
}

//AddClient adds a new client to the chat room
func (r *Room) AddClient(c io.ReadWriteCloser, clientname string, mode string) {
	r.Lock()
	defer r.Unlock()
	if _, ok := r.clients[clientname]; ok {
		log.Printf("Client %s already exist in chat room %s, existing...", clientname, r.name)
		return
	} else {
		log.Printf("Adding client %s \n", clientname)
		wc, done := StartClient(clientname, mode, r.Msgch, c, r.name)
		r.clients[clientname] = wc
		go func() {
			<-done
			log.Print("back in room")
			r.RemoveClientSync(clientname)
		}()
	}
}

//ClCount returns the number of clients in a chat room
func (r *Room) ClCount() int {
	return len(r.clients)
}

//RemoveClientSync removes a client from the chat room. This is a blocking call
func (r *Room) RemoveClientSync(name string) {
	log.Print("here")
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	r.Lock()
	defer r.Unlock()
	log.Printf("Removing client %s \n", name)
	delete(r.clients, name)
	for _, wc := range r.clients {
		go func(wc chan<- string) {
			wc <- "leavingroommate:" + name + "\n"
		}(wc.wc)
	}
}

//Run runs a chat room
func (r *Room) Run() {
	log.Println("Starting chat room", r.name)
	//handle the chat room BurpSuiteTeamServer message channel
	go func() {
		for msg := range r.Msgch {
			r.broadcastMsg(msg)
		}
	}()

	//handle when the quit channel is triggered
	go func() {
		<-r.Quit
		r.CloseChatRoomSync()
	}()
}

//CloseChatRoomSync closes a chat room. This is a blocking call
func (r *Room) CloseChatRoomSync() {
	r.Lock()
	defer r.Unlock()
	close(r.Msgch)
	for name := range r.clients {
		delete(r.clients, name)
	}
}

//fan out is used to distribute the chat message
func (r *Room) broadcastMsg(msg string) {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	r.RLock()
	defer r.RUnlock()
	sendingClient := strings.Split(msg, ":")[0]
	message := strings.Join(strings.Split(msg, ":")[1:], ":")
	log.Printf("Message: %s", message)
	if message == "newroommates\n" {
		keys := reflect.ValueOf(r.clients).MapKeys()
		strkeys := make([]string, len(keys))
		for i := 0; i < len(keys); i++ {
			strkeys[i] = keys[i].String()
		}
		log.Printf("Current clients: %s", strings.Join(strkeys, ","))
		for _, wc := range r.clients {
			go func(wc chan<- string) {
				wc <- "roommates:" + strings.Join(strkeys, ",") + "\n"
			}(wc.wc)
		}
	} else if strings.HasPrefix(message, "Repeater:") {
		messagePieces := strings.Split(message, ":")
		if messagePieces[1] == "To" {
			if index(r.clients[messagePieces[2]].mutedClients, sendingClient) == -1 {
				log.Printf("Sending burp repeater to specific client: %s", messagePieces[2])
				r.clients[messagePieces[2]].wc <- "Repeater:" + strings.Join(messagePieces[3:], ":")
			}
		} else {
			log.Printf("sending burp repeater to all client from %s \n", sendingClient)
			for clientName, wc := range r.clients {
				if index(wc.mutedClients, sendingClient) == -1 {
					if sendingClient != clientName {
						go func(wc chan<- string) {
							wc <- "Repeater:" + strings.Join(messagePieces[1:], ":")
						}(wc.wc)
					}
				}
			}
		}
	} else if strings.HasPrefix(message, "Intruder:") {
		messagePieces := strings.Split(message, ":")
		if messagePieces[1] == "To" {
			if index(r.clients[messagePieces[2]].mutedClients, sendingClient) == -1 {
				log.Printf("Sending burp intruder to specific client: %s", messagePieces[2])
				r.clients[messagePieces[2]].wc <- "Intruder:" + strings.Join(messagePieces[3:], ":")
			}
		} else {
			log.Printf("sending burp intruder to all client from %s \n", sendingClient)
			for clientName, wc := range r.clients {
				if index(wc.mutedClients, sendingClient) == -1 {
					if sendingClient != clientName {
						go func(wc chan<- string) {
							wc <- "Intruder:" + strings.Join(messagePieces[1:], ":")
						}(wc.wc)
					}
				}
			}
		}
	} else if strings.HasPrefix(message, "To:") {
		messagePieces := strings.Split(message, ":")
		if index(r.clients[messagePieces[2]].mutedClients, sendingClient) == -1 {
			log.Printf("Sending burp request to specific client: %s", messagePieces[1])
			r.clients[messagePieces[1]].wc <- strings.Join(messagePieces[1:], ":")
		}
	} else {
		log.Printf("sending client %s \n", sendingClient)
		for clientName, wc := range r.clients {
			if sendingClient != clientName {
				if index(wc.mutedClients, sendingClient) == -1 {
					go func(wc chan<- string) {
						wc <- msg
					}(wc.wc)
				}
			}
		}
	}
}
