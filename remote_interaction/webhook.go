package main_c

import (
	"crypto/tls"
	"fmt"
	"github.com/Rhymen/go-whatsapp"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

type waHandler struct {
	c *whatsapp.Conn
}

func (h *waHandler) HandleError(err error) {

	if e, ok := err.(*whatsapp.ErrConnectionFailed); ok {
		log.Printf("Connection failed, underlying error: %v", e.Err)
		log.Println("Waiting 30sec...")
		<-time.After(30 * time.Second)
		log.Println("Reconnecting...")
		err := h.c.Restore()
		if err != nil {
			log.Fatalf("Restore failed: %v", err)
		}
	} else {
		log.Printf("error occoured: %v\n", err)
	}
}

func (*waHandler) HandleTextMessage(message whatsapp.TextMessage) {
	webhook := os.Getenv("GETTING_MESSAGES_WEBHOOK")
	if webhook == "" {
		panic("Env var `GETTING_MESSAGES_WEBHOOK` not set")
	}

	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	resp, err := http.PostForm(webhook, url.Values{
		"timestamp": {strconv.FormatUint(message.Info.Timestamp, 10)},
		"message_id": {message.Info.Id},
		"message_jid": {message.Info.RemoteJid},
		"quoted_message_id": {message.Info.QuotedMessageID},
		"text": {message.Text},
	})

	if nil != err {
		fmt.Println("errorination happened getting the response", err)
		return
	}

	defer resp.Body.Close()
	_, err = ioutil.ReadAll(resp.Body)

	if nil != err {
		fmt.Println("errorination happened reading the body", err)
		return
	}
}


func gettingMessages() {
	wac, err := whatsapp.NewConn(20 * time.Second)
	if err != nil {
		log.Fatalf("error creating connection: %v\n", err)
	}

	//Add handler
	wac.AddHandler(&waHandler{wac})

	//login or restore
	if err := Login(wac); err != nil {
		log.Fatalf("error logging in: %v\n", err)
	}

	//verifies phone connectivity
	pong, err := wac.AdminTest()

	if !pong || err != nil {
		log.Fatalf("error pinging in: %v\n", err)
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	//Disconnect safe
	fmt.Println("Shutting down now.")
	session, err := wac.Disconnect()
	if err != nil {
		log.Fatalf("error disconnecting: %v\n", err)
	}
	if err := WriteSession(session, "test_name"); err != nil {
		log.Fatalf("error saving session: %v", err)
	}
}

func main()  {
	fmt.Println("Webhook running...")
	gettingMessages()
}
