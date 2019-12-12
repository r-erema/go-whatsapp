package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/Rhymen/go-whatsapp"
	"github.com/Rhymen/go-whatsapp/binary/proto"
	"github.com/bitly/go-simplejson"
	"github.com/go-redis/redis/v7"
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

const STATIC_DIR = "remote_interaction/static/"
const STATIC_URL_PATH = "/static/"

var redisClient *redis.Client

func main() {

	redisHost := os.Getenv("REDIS_HOST")
	if redisHost == "" {
		panic("Env var `REDIS_HOST` not set")
	}

	router := mux.NewRouter().StrictSlash(true)
	router.
		PathPrefix(STATIC_URL_PATH).
		Handler(http.StripPrefix(STATIC_URL_PATH, http.FileServer(http.Dir("./"+STATIC_DIR))))

	router.HandleFunc("/send-message/", sendMessage).Methods("POST")
	router.HandleFunc("/register-session/", registerSession).Methods("POST")
	router.HandleFunc("/get-qr-code/", getQrCode).Methods("GET")

	redisClient = redis.NewClient(&redis.Options{
		Addr: redisHost,
	})
	pong, err := redisClient.Ping().Result()
	if err != nil {
		log.Fatalf("Redis error: %v\n", err)
	}
	fmt.Println("Ping redis client:", pong)

	host := "0.0.0.0:443"
	log.Println("Api started listening", host, "...")
	err = http.ListenAndServeTLS(host, os.Getenv("CERT_FILE_PATH"), os.Getenv("CERT_KEY_PATH"), router)
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

func messageAlreadySent(messageId string) bool {
	_, err := redisClient.Get("wapi_sent_message:" + messageId).Result()
	if err != nil {
		return false
	}
	return true
}

func (handler *waHandler) HandleTextMessage(message whatsapp.TextMessage) {

	fmt.Println("Income message to wapi service, time:", time.Now().String())
	fmt.Println("Message:", message)
	fmt.Println()

	if messageAlreadySent(message.Info.Id) {
		fmt.Println("Attempt to send message which was sent earlier, time:", time.Now().String())
		fmt.Println("Message:", message)
		fmt.Println()
		return
	}

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

	err = redisClient.Set("wapi_sent_message:"+message.Info.Id, time.Now().String(), time.Hour*24*30).Err()
	if err != nil {
		fmt.Println("Can't store message id in redis", err)
	}
	fmt.Println("Message id stored in redis:", message.Info.Id)
	fmt.Println()

	fmt.Println("Message sent to", webhookUrl, "time", time.Now().String())
	fmt.Println("Message", message)
	fmt.Println()

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

	fmt.Println("gettingMessages called, time:", time.Now().String())
	fmt.Println()

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

	fmt.Println("sendMessage called, time:", time.Now().String())
	fmt.Println()

	decoder := json.NewDecoder(request.Body)
	var msgReq SendMessageRequest
	err := decoder.Decode(&msgReq)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't decode request: %v\n", err)
		return
	}

	wac, ok := wacs[msgReq.Session_name]

	if ok {
		if !wac.IsConnected() {
			wac, err_conn = whatsapp.NewConn(20 * time.Second)
			if err_conn != nil {
				fmt.Fprintf(os.Stderr, "error creating connection: %v\n", err_conn)
				return
			}
		}
	} else {
		wac, err_conn = whatsapp.NewConn(20 * time.Second)
		wacs[msgReq.Session_name] = wac
	}

	if !wac.Info.Connected {
		wac, err_conn = whatsapp.NewConn(20 * time.Second)
		if err_conn != nil {
			fmt.Fprintf(os.Stderr, "error creating connection: %v\n", err_conn)
			return
		}
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
			SenderJid:     wac.Info.Wid,
			QuotedMessage: *quotedMessage, //you also must send a valid QuotedMessageID
		},
		Text: msgReq.Text,
	}

	_, err = wac.Send(message)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error logging in: %v\n", err)
	}
	responseBody, err := json.Marshal(&message)
	responseWriter.Header().Set("Content-Type", "application/json")
	responseWriter.Write(responseBody)

	fmt.Println("Message sendeded, time:", time.Now().String())
	fmt.Println("Message:", message)
	fmt.Println()
}

var wacs = make(map[string]*whatsapp.Conn)

var err_conn error

func registerSession(responseWriter http.ResponseWriter, request *http.Request) {

	decoder := json.NewDecoder(request.Body)
	var msgReq RegisterSessionRequest
	err := decoder.Decode(&msgReq)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't decode request: %v\n", err)
		return
	}

	wac, ok := wacs[msgReq.SessionId]

	if ok {
		if !wac.IsConnected() {
			wac, err_conn = whatsapp.NewConn(20 * time.Second)
			if err_conn != nil {
				fmt.Fprintf(os.Stderr, "error creating connection: %v\n", err_conn)
				return
			}
		}
	} else {
		wac, err_conn = whatsapp.NewConn(20 * time.Second)
		wacs[msgReq.SessionId] = wac
	}

	/*if !ok || !wac.Info.Connected {
		wac, err_conn = whatsapp.NewConn(20 * time.Second)
		if err_conn != nil {
			fmt.Fprintf(os.Stderr, "error creating connection: %v\n", err_conn)
			return
		}
		wacs[msgReq.SessionId] = wac
	}*/

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
