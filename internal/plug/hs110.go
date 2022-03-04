package plug

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/pkg/errors"

	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

type Hs1xxPlug struct {
	IPAddress string
	Conn      *net.Conn
	locker    *sync.Mutex
}

type Command struct {
	System *System `json:"system"`
}

func (p *Hs1xxPlug) TurnOn() (err error) {
	json := `{"system":{"set_relay_state":{"state":1}}}`
	data := encrypt(json)
	_, err = p.send(data)
	return
}

func (p *Hs1xxPlug) TurnOff() (err error) {
	json := `{"system":{"set_relay_state":{"state":0}}}`
	data := encrypt(json)
	_, err = p.send(data)
	return
}

func (p *Hs1xxPlug) SetState(childId string, state bool) (err error) {
	var numState int
	if state {
		numState = 1
	}
	//log.Println("setting state: ", numState)
	jsn := fmt.Sprintf(`{"context": {"child_ids": ["%s"]}, "system":{"set_relay_state":{"state":%d}}}`, childId, numState)
	data := encrypt(jsn)
	_, err = p.send(data)
	return
}

func (p *Hs1xxPlug) SystemInfo() ([]byte, error) {
	cmd := Command{System: &System{GetSysinfo: &GetSysinfo{}}}
	buf := new(bytes.Buffer)
	err := json.NewEncoder(buf).Encode(&cmd)
	if err != nil {
		return nil, err
	}
	data := encrypt(strings.TrimSpace(buf.String()))
	reading, err := p.send(data)
	if err == nil {
		return decrypt(reading), nil
	}
	return nil, err
}

func (p *Hs1xxPlug) MeterInfo(childIds []string) ([]byte, error) {
	json := `{"emeter":{"get_realtime":{}}}`
	if len(childIds) > 0 {
		json = `{"context": {"child_ids": ["` + strings.Join(childIds, "\",\"") +
			`"]},  "emeter":{"get_realtime":{}}}`
	}
	//fmt.Println(json)
	data := encrypt(json)
	reading, err := p.send(data)
	if err == nil {
		results := decrypt(reading)
		return results, nil
	}
	return nil, err
}

func (p *Hs1xxPlug) DailyStats(month int, year int) (results []byte, err error) {
	json := fmt.Sprintf(`{"emeter":{"get_daystat":{"month":%d,"year":%d}}}`, month, year)
	data := encrypt(json)
	reading, err := p.send(data)
	if err == nil {
		results = decrypt(reading[4:])
	}
	return
}

func encrypt(plaintext string) []byte {
	n := len(plaintext)
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, uint32(n))
	if err != nil {
		panic(err)
	}
	ciphertext := buf.Bytes()

	key := byte(0xAB)
	payload := make([]byte, n)
	for i := 0; i < n; i++ {
		payload[i] = plaintext[i] ^ key
		key = payload[i]
	}

	for i := 0; i < len(payload); i++ {
		ciphertext = append(ciphertext, payload[i])
	}

	return ciphertext
}

func decrypt(ciphertext []byte) []byte {
	n := len(ciphertext)
	key := byte(0xAB)
	var nextKey byte
	for i := 0; i < n; i++ {
		nextKey = ciphertext[i]
		ciphertext[i] = ciphertext[i] ^ key
		key = nextKey
	}
	return ciphertext
}

func (p *Hs1xxPlug) ReopenConnection() error {
	p.locker.Lock()
	defer p.locker.Unlock()
	if p.Conn != nil {
		err := (*p.Conn).Close()
		if err != nil {
			log.Err(err).Msg("Failed to close connection")
		}
	}
	con, err := net.DialTimeout("tcp", p.IPAddress+":9999", time.Duration(5)*time.Second)

	if err != nil {
		return errors.Wrap(err, "Failed to dial plug")
	}
	if tc, ok := con.(*net.TCPConn); ok {
		err := tc.SetKeepAlive(true)
		if err != nil {
			return errors.Wrap(err, "Failed to set keep alive")
		}

		err = tc.SetKeepAlivePeriod(60 * time.Second)
		if err != nil {
			return errors.Wrap(err, "Failed to set keep alive period")
		}
	}
	p.Conn = &con
	return nil
}

func NewPlug(ip string) (Hs1xxPlug, error) {
	var plug Hs1xxPlug
	plug.IPAddress = ip
	plug.locker = &sync.Mutex{}
	return plug, plug.ReopenConnection()
}

func (p *Hs1xxPlug) send(payload []byte) ([]byte, error) {
	p.locker.Lock()
	defer p.locker.Unlock()

	_, err := (*p.Conn).Write(payload)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to write payload")
	}
	buf := new(bytes.Buffer)

	n, err := io.CopyN(buf, *p.Conn, 4)
	if err != nil {
		log.Debug().Str("buf", buf.String()).Msg("Existing read buffer")
		return nil, errors.Wrap(err, "Failed to read connection")
	}
	if n != 4 {
		return nil, errors.New("Failed to read message size")
	}
	numOfBytes := binary.BigEndian.Uint32(buf.Bytes())
	if numOfBytes == 0 {
		return nil, errors.New("No response to read")
	}

	buf = new(bytes.Buffer)

	_, err = io.CopyN(buf, *p.Conn, int64(numOfBytes))
	return buf.Bytes(), err
}

type GetSysinfo struct {
	SwVer      string  `json:"sw_ver,omitempty"`
	HwVer      string  `json:"hw_ver,omitempty"`
	Model      string  `json:"model,omitempty"`
	DeviceID   string  `json:"deviceId,omitempty"`
	OemID      string  `json:"oemId,omitempty"`
	HwID       string  `json:"hwId,omitempty"`
	Rssi       int     `json:"rssi,omitempty"`
	LongitudeI int     `json:"longitude_i,omitempty"`
	LatitudeI  int     `json:"latitude_i,omitempty"`
	Alias      string  `json:"alias,omitempty"`
	Status     string  `json:"status,omitempty"`
	MicType    string  `json:"mic_type,omitempty"`
	Feature    string  `json:"feature,omitempty"`
	Mac        string  `json:"mac,omitempty"`
	Updating   int     `json:"updating,omitempty"`
	LedOff     int     `json:"led_off,omitempty"`
	Children   []Child `json:"children,omitempty"`
	ChildNum   int     `json:"child_num,omitempty"`
	ErrCode    int     `json:"err_code,omitempty"`
}

type Child struct {
	ID         string `json:"id"`
	State      int    `json:"state"`
	Alias      string `json:"alias"`
	OnTime     int    `json:"on_time"`
	NextAction struct {
		Type int `json:"type"`
	} `json:"next_action"`
}

type System struct {
	GetSysinfo *GetSysinfo `json:"get_sysinfo"`
}

type Emeter struct {
	GetRealtime struct {
		VoltageMv int `json:"voltage_mv"`
		CurrentMa int `json:"current_ma"`
		PowerMw   int `json:"power_mw"`
		TotalWh   int `json:"total_wh"`
		ErrCode   int `json:"err_code"`
	} `json:"get_realtime"`
	GetVgainIgain struct {
		ErrCode int    `json:"err_code"`
		ErrMsg  string `json:"err_msg"`
	} `json:"get_vgain_igain"`
}

type PowerInfo struct {
	Emeter Emeter `json:"emeter"`
}

type SystemInfo struct {
	System System `json:"system"`
}

type SystemWithPower struct {
	Emeter Emeter `json:"emeter"`
	System System `json:"system"`
}
