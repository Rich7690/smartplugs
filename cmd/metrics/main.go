package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Rich7690/plugstats/internal/plug"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	//go:embed resources/*
	res   embed.FS
	pages = map[string]string{
		"/":       "resources/index.html",
		"/lib.js": "resources/lib.js",
	}
)

var powerMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: "plugs",
	Name:      "current_power",
	Help:      "",
}, []string{"alias", "plug_ip"})

var errMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: "plugs",
	Name:      "current_error",
	Help:      "",
}, []string{"plug_ip"})

var lock = &sync.RWMutex{}
var infoMap = make(map[string]plug.SystemInfo)
var plugMap = make(map[string]plug.Hs1xxPlug)
var upgrader = websocket.Upgrader{}

func main() {
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs

	upgrader.EnableCompression = true
	prometheus.MustRegister(powerMetric, errMetric)

	ips := os.Getenv("IP_ADDR")
	if ips == "" {
		log.Fatal().Msg("No ips configured")
	}
	ipList := strings.Split(ips, ":")

	for i := range ipList {
		i := i
		go func() {
			ip := ipList[i]
			plg, err := plug.NewPlug(ip)
			if err != nil {
				log.Fatal().Err(err).Msg("Failed to create plug")
			}
			res, err := plg.SystemInfo()
			if err == nil {
				var sys plug.SystemInfo
				err = json.NewDecoder(bytes.NewBuffer(res)).Decode(&sys)
				if err == nil {
					infoMap[sys.System.GetSysinfo.DeviceID] = sys
					plugMap[sys.System.GetSysinfo.DeviceID] = plg
				}
				log.Debug().Str("alias", sys.System.GetSysinfo.Alias).Msg("Discovered device")
			}
			for {
				time.Sleep(10 * time.Second)
				sys, err := refreshMap(plg)
				if err != nil {
					errMetric.WithLabelValues(ip).Set(1)
					log.Debug().Msg("Reopening connection")
					err := plg.ReopenConnection()
					if err != nil {
						log.Err(err).Msg("Failed to reopen")
					}
					continue
				} else {
					errMetric.WithLabelValues(ip).Set(0)
				}

				err = refreshPower(plg, sys)
				if err != nil {
					errMetric.WithLabelValues(ip).Set(1)
					log.Debug().Msg("Reopening connection")
					err := plg.ReopenConnection()
					if err != nil {
						log.Err(err).Msg("Failed to reopen")
					}
				}
			}
		}()
	}

	ro := mux.NewRouter()
	ro.StrictSlash(true)
	ro.Path("/metrics").Methods(http.MethodGet).Handler(promhttp.Handler())
	ro.Path("/health").Methods(http.MethodGet).HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	ro.Path("/socket").HandlerFunc(handleSocket)
	//fs := http.FileServer(http.FS((res)))
	//ro.PathPrefix("/static").Methods(http.MethodGet).Handler(fs)

	//ro.PathPrefix("/").Methods(http.MethodGet).Handler(fs)
	ro.PathPrefix("/").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, ok := pages[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		b, err := res.ReadFile(page)
		if err != nil {
			log.Printf("page %s not found in pages cache...", r.RequestURI)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", mime.TypeByExtension(filepath.Ext(page)))
		w.Write(b)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "9091"
	}
	ro.Use(func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			log.Debug().Str("path", r.URL.Path).Str("method", r.Method).Msg("http request")
			h.ServeHTTP(rw, r)
		})
	})
	err := http.ListenAndServe(":"+port, ro)
	if err != http.ErrServerClosed {
		log.Err(err).Msg("Error starting web server")
	}
}

type ClientMsg struct {
	Type   string                 `json:"type"`
	Params map[string]interface{} `json:"params"`
}

func handleSocket(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Err(err).Msg("error upgrading connection")
		return
	}
	c.EnableWriteCompression(true)
	defer c.Close()
	_ = c.WriteJSON(&infoMap)
	msgChan := make(chan ClientMsg, 10)
	go func() {
		for {
			var msg ClientMsg
			err := c.ReadJSON(&msg)
			if websocket.IsCloseError(err, websocket.CloseGoingAway) {
				log.Info().Msg("Got close 1001 when trying to read json")
				return
			} else if err != nil {
				log.Err(err).Msg("error reading message")
				return
			}
			msgChan <- msg
		}
	}()
	for {
		select {
		case msg := <-msgChan:
			switch msg.Type {
			case "TOGGLE":
				err = handleToggle(msg.Params)
				if err != nil {
					log.Err(err).Msg("error toggling")
				}
				err = c.WriteJSON(&infoMap)
				if errors.Is(err, websocket.ErrCloseSent) {
					log.Info().Msg("Got close sent when trying to write json")
				} else if err != nil {
					log.Err(err).Msg("error writing json")
				}
			}
		case <-time.After(10 * time.Second):
			err := c.WriteJSON(&infoMap)
			if err != nil {
				log.Err(err).Msg("error writing json")
				return
			}
		}
	}
}

func handleToggle(params map[string]interface{}) error {
	id, ok := params["id"].(string)
	if !ok {
		return errors.New("Bad params. Missing id")
	}
	val, ok := infoMap[id]
	if !ok {
		return errors.New("id Not found")
	}
	childId, ok := params["childId"].(string)
	if !ok {
		return errors.New("Bad params. Missing childId")
	}

	var child *plug.Child

	for i := range val.System.GetSysinfo.Children {
		if val.System.GetSysinfo.Children[i].ID == childId {
			child = &val.System.GetSysinfo.Children[i]
			break
		}
	}
	if child == nil {
		return errors.New("id Not found")
	}

	var newState bool
	if child.State == 0 {
		newState = true
	}

	plg := plugMap[id]

	err := plg.SetState(child.ID, newState)
	if err != nil {
		return err
	}
	_, err = refreshMap(plg)
	return err
}

func refreshPower(plg plug.Hs1xxPlug, sys plug.SystemInfo) error {
	var retErr error = nil
	for i := range sys.System.GetSysinfo.Children {
		res, err := plg.MeterInfo([]string{sys.System.GetSysinfo.Children[i].ID})
		if err != nil {
			log.Err(err).Str("alias", sys.System.GetSysinfo.Children[i].Alias).Msg("error reading message")
			retErr = err
		}

		buf := bytes.NewBuffer(res)
		var power plug.PowerInfo
		err = json.NewDecoder(buf).Decode(&power)
		if err != nil {
			log.Err(err).Msg("err decoding power info")
			retErr = err
		}
		powerMetric.WithLabelValues(sys.System.GetSysinfo.Children[i].Alias, plg.IPAddress).Set(float64(power.Emeter.GetRealtime.PowerMw))
	}
	return retErr
}

func refreshMap(plg plug.Hs1xxPlug) (plug.SystemInfo, error) {
	var sys plug.SystemInfo
	lock.Lock()
	defer lock.Unlock()
	res, err := plg.SystemInfo()
	if err != nil {
		log.Err(err).Msg("failed to get system info")
		return sys, err
	}

	buf := bytes.NewBuffer(res)

	err = json.NewDecoder(buf).Decode(&sys)
	if err != nil {
		log.Err(err).Bytes("buf", res).Str("json", string(res)).Msg("failed to decode json")
		return sys, err
	}

	infoMap[sys.System.GetSysinfo.DeviceID] = sys
	return sys, nil
}
