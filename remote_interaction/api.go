package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/Rhymen/go-whatsapp"
	"github.com/Rhymen/go-whatsapp/binary/proto"
	"github.com/bitly/go-simplejson"
	"github.com/gorilla/mux"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

var apiHost = os.Getenv("WAPI_HOST")

const STATIC_DIR = "remote_interaction/static/"
const STATIC_URL_PATH = "/static/"

func main() {

	if apiHost == "" {
		log.Fatalf("Env var `WAPI_HOST` not set")
	}

	router := mux.NewRouter().StrictSlash(true)
	router.
		PathPrefix(STATIC_URL_PATH).
		Handler(http.StripPrefix(STATIC_URL_PATH, http.FileServer(http.Dir("./"+STATIC_DIR))))

	router.HandleFunc("/send-message/", sendMessage).Methods("POST")
	router.HandleFunc("/register-session/", registerSession).Methods("POST")
	router.HandleFunc("/get-qr-code/", getQrCode).Methods("GET")

	log.Println("Api started listening", apiHost, "...")
	err := http.ListenAndServe(os.Getenv("WAPI_HOST"), router)
	if err != nil {
		log.Fatalf("error saving session: %v\n", err)
	}
}

type waHandler struct {
	c             *whatsapp.Conn
	sessionName   string
	initTimestamp uint64
}

func (handler *waHandler) HandleError(err error) {

	if e, ok := err.(*whatsapp.ErrConnectionFailed); ok {
		log.Printf("Connection failed, underlying error: %v", e.Err)
		log.Println("Waiting 30sec...")
		<-time.After(30 * time.Second)
		log.Println("Reconnecting...")
		err := handler.c.Restore()
		if err != nil {
			log.Fatalf("Restore failed: %v", err)
		}
	} else {
		log.Printf("error occoured: %v\n", err)
	}
}

func (handler *waHandler) HandleTextMessage(message whatsapp.TextMessage) {

	if handler.initTimestamp == 0 {
		handler.initTimestamp = uint64(time.Now().Unix())
	}

	if message.Info.Timestamp <= handler.initTimestamp {
		return
	}

	if message.Info.FromMe {
		return
	}

	webhook := os.Getenv("GETTING_MESSAGES_WEBHOOK")
	if webhook == "" {
		panic("Env var `GETTING_MESSAGES_WEBHOOK` not set")
	}

	webhookUrl := webhook + handler.sessionName
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	requestBody, err := json.Marshal(&message)
	resp, err := http.Post(webhookUrl, "application/json", bytes.NewBuffer(requestBody))

	if nil != err {
		fmt.Println("errorination happened getting the response", err)
		return
	}
	fmt.Println("Message sent to", webhookUrl)
	fmt.Println("Message", message)

	defer resp.Body.Close()
	_, err = ioutil.ReadAll(resp.Body)

	if nil != err {
		fmt.Println("errorination happened reading the body", err)
		return
	}
}

type SendMessageRequest struct {
	Chat_id      string `json:"chat_id"`
	Text         string `json:"text"`
	Session_name string `json: session_name`
}

type RegisterSessionRequest struct {
	SessionId string `json:"session_id"`
}

func gettingMessages(wac *whatsapp.Conn, sessionName string) {

	wac.AddHandler(&waHandler{wac, sessionName, 0})

	/*pong, err := wac.AdminTest()

	if !pong || err != nil {
		log.Fatalf("error pinging in: %v\n", err)
	}*/

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	//Disconnect safe
	fmt.Println("Shutting down now.")
	session, err := wac.Disconnect()
	if err != nil {
		log.Fatalf("error disconnecting: %v\n", err)
	}
	if err := WriteSession(session, sessionName); err != nil {
		log.Fatalf("error saving session: %v", err)
	}
}

func sendMessage(responseWriter http.ResponseWriter, request *http.Request) {

	if !wac.Info.Connected {
		wac, err_conn = whatsapp.NewConn(20 * time.Second)
		if err_conn != nil {
			fmt.Fprintf(os.Stderr, "error creating connection: %v\n", err_conn)
			return
		}
	}

	decoder := json.NewDecoder(request.Body)
	var msgReq SendMessageRequest
	error := decoder.Decode(&msgReq)
	if error != nil {
		fmt.Errorf("Can't decode request: %v\n", error)
	}

	if !wac.IsLoggedIn() {
		err := Login(wac, msgReq.Session_name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error logging in: %v\n", err)
			return
		}
	}

	<-time.After(3 * time.Second)

	previousMessage := "😘"
	quotedMessage := &proto.Message{
		Conversation: &previousMessage,
	}

	message := whatsapp.TextMessage{
		Info: whatsapp.MessageInfo{
			RemoteJid:     msgReq.Chat_id,
			QuotedMessage: *quotedMessage, //you also must send a valid QuotedMessageID
		},
		Text: msgReq.Text,
	}

	wac.Send(message)
}

var wac = &whatsapp.Conn{Info: &whatsapp.Info{Connected: false}}
var err_conn error

func registerSession(responseWriter http.ResponseWriter, request *http.Request) {

	if !wac.Info.Connected {
		wac, err_conn = whatsapp.NewConn(20 * time.Second)
		if err_conn != nil {
			fmt.Fprintf(os.Stderr, "error creating connection: %v\n", err_conn)
			return
		}
	}

	decoder := json.NewDecoder(request.Body)
	var msgReq RegisterSessionRequest
	err := decoder.Decode(&msgReq)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't decode request: %v\n", err)
		return
	}

	if !wac.IsLoggedIn() {
		err = Login(wac, msgReq.SessionId)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error logging in: %v\n", err)
			return
		}
	}

	json := simplejson.New()
	json.Set("ok", true)

	payload, err := json.MarshalJSON()
	if err != nil {
		log.Println(err)
	}

	responseWriter.Header().Set("Content-Type", "application/json")
	responseWriter.Write(payload)

	if err != nil {
		fmt.Fprintf(os.Stderr, "error logging in: %v\n", err)
		return
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		wg.Done()
		gettingMessages(wac, msgReq.SessionId)
	}()
	wg.Wait()
}

func getQrCode(responseWriter http.ResponseWriter, request *http.Request) {
	/*
		responseWriter.Header().Set("Content-Type", mime.TypeByExtension(filepath.Ext(file)))
		http.ServeFile(responseWriter, request, file)*/

	vars := request.URL.Query()
	val, ok := vars["session_name"]
	if ok {
		sessionName := val[0]
		qrImgPath := resolveQrCodesURLPath(sessionName)
		t := template.Must(template.ParseFiles("remote_interaction/static/qr-code.html"))
		data := struct{ QrCodeImgPath, SessionName string }{qrImgPath, sessionName}
		if err := t.ExecuteTemplate(responseWriter, "qr-code.html", data); err != nil {
			http.Error(responseWriter, err.Error(), http.StatusInternalServerError)
		}
	}

}
