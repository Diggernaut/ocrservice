package main

import (
	"strings"
	"encoding/json"
	b64 "encoding/base64"
	"log"
	"net/http"

	"github.com/Diggernaut/cast"
	"github.com/Diggernaut/viper"
	"github.com/gorilla/mux"
	"github.com/natefinch/lumberjack"
	"github.com/Diggernaut/gosseract"
)

var (
	cfg    *viper.Viper
	apikey string
)

func init() {
	// SET UP LOGGER
	log.SetOutput(&lumberjack.Logger{
		Filename:   "/var/log/ocrservice.log",
		MaxSize:    100, // megabytes
		MaxBackups: 3,   // max files
		MaxAge:     7,   // days
	})
	//log.SetOutput(os.Stdout)

	// READING CONFIG
	cfg = viper.New()
	cfg.SetConfigName("config")
	cfg.AddConfigPath("./")
	err := cfg.ReadInConfig()
	if err != nil {
		log.Fatalf("Error: cannot read config. Reason: %v\n", err)
	}
	apikey = cfg.GetString("apikey")
}

func main() {
	log.Println("OCR web server started")
	router := mux.NewRouter()
	router.HandleFunc(`/base64`, base64).Methods("POST")
	sslCert := cfg.GetString("ssl_cert")
	privateKey := cfg.GetString("private_key")
	if sslCert != "" && privateKey != "" {
		err := http.ListenAndServeTLS(cfg.GetString("ocr_service_bind_ip")+":"+cfg.GetString("ocr_service_bind_port"), sslCert, privateKey, router)
		if err != nil {
			log.Fatalln(err)
		}
	} else {
		err := http.ListenAndServe(cfg.GetString("ocr_service_bind_ip")+":"+cfg.GetString("ocr_service_bind_port"), router)
		if err != nil {
			log.Fatalln(err)
		}
	}
}

func base64(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	defer finishHandle(&w)
	// SETTING UP RESPONSE HEADERS: CORS AND CONTENT-TYPE
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if r.Header.Get("Diggernauth") != apikey {
		errorResponse(403, "Invalid API key", &w)
		return
	}

	var body = new(struct {
		Base64    string                `json:"base64"`
		Trim      string                `json:"trim"`
		Languages string                `json:"languages"`
		Whitelist string                `json:"whitelist"`
		PSM       gosseract.PageSegMode `json:"psm"`
	})

	err := json.NewDecoder(r.Body).Decode(body)
	if err != nil {
		errorResponse(400, err.Error(), &w)
		return
	}

	if len(body.Base64) == 0 {
		errorResponse(400, "base64 string required", &w)
		return
	}
	b, err := b64.StdEncoding.DecodeString(body.Base64)
	if err != nil {
		errorResponse(400, err.Error(), &w)
		return
	}

	client := gosseract.NewClient()
	if body.PSM > 0 {
		client.SetPageSegMode(body.PSM)
	}
	defer func() {
		err := client.Close()
		if err != nil {
			log.Println("cannot close client, reason: %s", err.Error())
		}
	}()
	client.Languages = []string{"eng"}
	if body.Languages != "" {
		client.Languages = strings.Split(body.Languages, ",")
	}
	client.SetImageFromBytes(b)
	if body.Whitelist != "" {
		client.SetWhitelist(body.Whitelist)
	}

	text, err := client.Text()
	if err != nil {
		errorResponse(400, err.Error(), &w)
		return
	}

	response := make(map[string]interface{})
	response["status"] = "success"
	response["result"] = strings.Trim(text, body.Trim)
	bytedata, _ := json.Marshal(response)
	w.WriteHeader(http.StatusOK)
	w.Write(bytedata)
}

func errorResponse(code int, error string, w *http.ResponseWriter) {
	response := make(map[string]string)
	response["status"] = "failure"
	response["error"] = error
	bytedata, _ := json.Marshal(response)
	http.Error(*w, string(bytedata), code)
}

func finishHandle(w *http.ResponseWriter) {
	if x := recover(); x != nil {
		log.Printf("Run time panic: %v\n", x)
		errorResponse(500, cast.ToString(x), w)
	}
}
