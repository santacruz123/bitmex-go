package bitmex

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"github.com/apex/log"

	"golang.org/x/net/websocket"
)

const wsURL = "wss://www.bitmex.com/realtime"

//WS - websocket connection object
type WS struct {
	sync.Mutex
	conn    *websocket.Conn
	log     *log.Logger
	chTrade map[chan WSTrade][]Contracts
	chQuote map[chan WSQuote][]Contracts
}

//NewWS - creates new websocket object
func NewWS() *WS {
	return &WS{
		chTrade: make(map[chan WSTrade][]Contracts, 0),
		chQuote: make(map[chan WSQuote][]Contracts, 0),
	}
}

//Connect - connects
func (ws *WS) Connect() error {
	conn, err := websocket.Dial(wsURL, "", "http://localhost/")

	if err != nil {
		return err
	}

	log.Info("Connected")

	ws.conn = conn

	go ws.read()

	return nil
}

//Disconnect - Disconnects from websocket
func (ws *WS) Disconnect() error {
	log.Info("Disconnecting")
	//TODO Close all channels
	return ws.conn.Close()
}

func (ws *WS) read() {
	for {
		var msg string
		websocket.Message.Receive(ws.conn, &msg)

		log.Debugf("Raw: %v", msg)

		switch {
		case strings.HasPrefix(msg, `{"success"`):
			var success wsSuccess
			json.Unmarshal([]byte(msg), &success)
			log.Debugf("Success: %v", success)

		case strings.HasPrefix(msg, `{"info"`):
			var info wsInfo
			json.Unmarshal([]byte(msg), &info)
			log.Infof("Info: %v", info)

		case strings.Contains(msg, `{"table"`):
			var table wsData
			json.Unmarshal([]byte(msg), &table)
			log.Debugf("Table: %#v", table)

			switch table.Table {
			case "trade":
				var trades []WSTrade
				json.Unmarshal(table.Data, &trades)

				log.Debugf("Trades: %#v", trades)

				for _, one := range trades {
					ws.trade(one)
				}
			case "quote":
				var quotes []WSQuote
				json.Unmarshal(table.Data, &quotes)

				log.Debugf("Quotes: %#v", quotes)

				for _, one := range quotes {
					ws.quote(one)
				}
			}
		default:
			ws.fatal(errors.New("Unkown WS message"))
		}
	}
}

func (ws *WS) sendTrade(ch chan WSTrade, trade WSTrade) {
	select {
	case ch <- trade:
		log.Debugf("Trade sent: %#v - %#v", ch, trade)
	default:
		log.Debugf("Trade channel deleted: %#v", ch)
		ws.Lock()
		delete(ws.chTrade, ch)
		ws.Unlock()
	}
}

func (ws *WS) sendQuote(ch chan WSQuote, quote WSQuote) {
	select {
	case ch <- quote:
		log.Debugf("Quote sent: %#v - %#v", ch, quote)
	default:
		log.Debugf("Quote channel deleted: %#v", ch)
		ws.Lock()
		delete(ws.chQuote, ch)
		ws.Unlock()
	}
}

func (ws *WS) trade(trade WSTrade) {
	for ch, symbols := range ws.chTrade {
		if len(symbols) == 0 {
			ws.sendTrade(ch, trade)
			continue
		}

		for _, oneSymbol := range symbols {
			if oneSymbol == Contracts(trade.Symbol) {
				ws.sendTrade(ch, trade)
			}
		}
	}
}

func (ws *WS) quote(quote WSQuote) {
	for ch, symbols := range ws.chQuote {
		if len(symbols) == 0 {
			ws.sendQuote(ch, quote)
			continue
		}

		for _, oneSymbol := range symbols {
			if oneSymbol == Contracts(quote.Symbol) {
				ws.sendQuote(ch, quote)
			}
		}
	}
}

func (ws *WS) send(msg string) {
	defer ws.Unlock()

	log.Debugf("Writing WS: %#v", string(msg))
	ws.Lock()

	if _, err := ws.conn.Write([]byte(msg)); err != nil {
		ws.fatal(err)
	}
}

func (ws *WS) fatal(err error) {
	ws.Disconnect()
	log.Fatalf("%v", err.Error())
}

//SubTrade - subscribes channel to trades
func (ws *WS) SubTrade(ch chan WSTrade, contracts []Contracts) {
	ws.Lock()

	if _, ok := ws.chTrade[ch]; !ok {
		ws.chTrade[ch] = contracts
	} else {
		ws.chTrade[ch] = append(ws.chTrade[ch], contracts...)
	}

	ws.Unlock()

	for _, one := range contracts {
		ws.send(`{"op": "subscribe", "args": "trade:` + string(one) + `"}`)
	}
}

func (ws *WS) SubQuote(ch chan WSQuote, contracts []Contracts) {

	ws.Lock()

	if _, ok := ws.chQuote[ch]; !ok {
		ws.chQuote[ch] = contracts
	} else {
		ws.chQuote[ch] = append(ws.chQuote[ch], contracts...)
	}

	ws.Unlock()

	for _, one := range contracts {
		ws.send(`{"op": "subscribe", "args": "quote:` + string(one) + `"}`)
	}
}
