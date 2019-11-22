package main

import (
	"encoding/gob"
	"fmt"
	"github.com/Rhymen/go-whatsapp"
	"github.com/skip2/go-qrcode"
	"net/url"
	"os"
)

func Login(wac *whatsapp.Conn, sessionName string) error {
	//load saved session
	fmt.Println("Logging...")
	session, err := readSession(sessionName)
	if err == nil {
		//restore session
		session, err = wac.RestoreWithSession(session)
		if err != nil {
			return fmt.Errorf("restoring failed: %v\n", err)
		}
	} else {
		//no saved session -> regular login
		qr := make(chan string)
		go func() {
			err = qrcode.WriteFile(<-qr, qrcode.Medium, 256, resolveQrCodesFilePath(sessionName))
			if err != nil {
				_ = fmt.Errorf("can't create qr code: %v\n", err)
			}
			qrUrl := url.URL{
				Scheme:   "http",
				Host:     os.Getenv("WAPI_URL"),
				Path:     "get-qr-code",
				RawQuery: "session_name=" + sessionName,
			}
			fmt.Println("Please, scan QR code in next url to log in:", qrUrl.String())
		}()
		session, err = wac.Login(qr)
		if err != nil {
			return fmt.Errorf("error during login: %v\n", err)
		}
	}

	//save session
	err = WriteSession(session, sessionName)
	if err != nil {
		return fmt.Errorf("error saving session: %v\n", err)
	}
	fmt.Println("Logging success")
	return nil
}

func resolveSessionFilePath(sessionName string) string {
	return os.TempDir() + "/" + sessionName + ".gob"
}

func resolveQrCodesFilePath(sessionName string) string {
	return STATIC_DIR + "qr-codes/qr_" + sessionName + ".png"
}
func resolveQrCodesURLPath(sessionName string) string {
	return STATIC_URL_PATH + "qr-codes/qr_" + sessionName + ".png"
}

func WriteSession(session whatsapp.Session, sessionName string) error {
	file, err := os.Create(resolveSessionFilePath(sessionName))
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := gob.NewEncoder(file)
	err = encoder.Encode(session)
	if err != nil {
		return err
	}
	return nil
}

func readSession(sessionName string) (whatsapp.Session, error) {
	session := whatsapp.Session{}
	file, err := os.Open(resolveSessionFilePath(sessionName))
	if err != nil {
		return session, err
	}
	defer file.Close()
	decoder := gob.NewDecoder(file)
	err = decoder.Decode(&session)
	if err != nil {
		return session, err
	}
	return session, nil
}
