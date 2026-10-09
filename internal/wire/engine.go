// Package wire translates ECHONET Lite frames into model operations.
package wire

import (
	"bytes"
	"fmt"
	"sync"

	"github.com/cybergarage/uecho-go/net/echonet/protocol"
	"github.com/cybergarage/uecho-simulator/internal/model"
)

type Engine struct {
	mu    sync.Mutex
	store *model.Store
	tid   uint
}

func New(s *model.Store) *Engine { return &Engine{store: s} }

// Handle is the single-instance convenience API. UDP uses HandleAll so wildcard
// requests can produce one response for every matching concrete instance.
func (e *Engine) Handle(b []byte, source string) ([]byte, error) {
	all, err := e.HandleAll(b, source)
	if err != nil || len(all) == 0 {
		return nil, err
	}
	return all[0], nil
}
func (e *Engine) HandleAll(b []byte, source string) ([][]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.handleAll(b, source, false)
}
func (e *Engine) handleAll(b []byte, source string, simulated bool) ([][]byte, error) {
	rx, tx := "RX", "TX"
	if simulated {
		rx, tx = "SIM-RX", "SIM-TX"
	}
	req, get, err := parseRequest(b)
	if err != nil {
		e.store.Log(rx, source, "rejected frame", b)
		e.store.Log("ERROR", source, err.Error(), nil)
		return nil, err
	}
	esv := req.ESV()
	if esv != 0x60 && esv != 0x61 && esv != 0x62 && esv != 0x63 && esv != 0x6e && esv != 0x74 {
		return nil, nil
	}
	e.store.Log(rx, source, "request", b)
	out := [][]byte{}
	for _, target := range e.store.Targets(uint32(req.DEOJ())) {
		res := protocol.NewResponseMessageWithMessage(req)
		res.SetSEOJ(protocol.ObjectCode(target))
		failed := false
		var getRes *protocol.Message
		if get != nil {
			getRes = protocol.NewResponseMessageWithMessage(get)
			getRes.SetSEOJ(protocol.ObjectCode(target))
		}
		appendAnswer := func(message *protocol.Message, p protocol.Property, read bool) {
			answer := protocol.NewPropertyWithCode(p.Code())
			if esv == 0x74 {
				message.AddProperty(answer)
				return
			}
			if read {
				data, readErr := e.store.Read(target, byte(p.Code()))
				if p.Size() != 0 {
					readErr = fmt.Errorf("read requires empty EDT")
				}
				if readErr != nil {
					failed = true
					e.store.Log("ERROR", source, readErr.Error(), nil)
				} else {
					answer.SetData(data)
				}
			} else {
				if writeErr := e.store.Write(target, byte(p.Code()), p.Data(), source); writeErr != nil {
					failed = true
					answer.SetData(p.Data())
					e.store.Log("ERROR", source, writeErr.Error(), nil)
				}
			}
			message.AddProperty(answer)
		}
		for _, p := range req.Properties() {
			appendAnswer(res, p, esv == 0x62 || esv == 0x63)
		}
		if get != nil {
			for _, p := range get.Properties() {
				appendAnswer(getRes, p, true)
			}
		}
		if esv == 0x60 && !failed {
			continue
		}
		if failed {
			res.SetESV(protocol.ESV(byte(esv) - 0x10))
		}
		if esv == 0x60 {
			res.SetESV(0x50)
		}
		// A bounded response reports the processed prefix rather than emitting an
		// oversized datagram. SetGet_SNA may contain zero properties in either block.
		if len(combinedBytes(res, getRes)) > MaxFrameSize {
			res.SetESV(protocol.ESV(byte(esv) - 0x10))
			first := protocol.NewResponseMessageWithMessage(req)
			first.SetSEOJ(protocol.ObjectCode(target))
			first.SetESV(res.ESV())
			var second *protocol.Message
			if getRes != nil {
				second = protocol.NewResponseMessageWithMessage(get)
				second.SetSEOJ(protocol.ObjectCode(target))
				second.SetESV(res.ESV())
			}
			budget := MaxFrameSize - 12
			if second != nil {
				budget--
			}
			for _, p := range res.Properties() {
				cost := 2 + p.Size()
				if cost > budget {
					break
				}
				first.AddProperty(p)
				budget -= cost
			}
			if second != nil {
				for _, p := range getRes.Properties() {
					cost := 2 + p.Size()
					if cost > budget {
						break
					}
					second.AddProperty(p)
					budget -= cost
				}
			}
			res, getRes = first, second
		}
		encoded := combinedBytes(res, getRes)
		out = append(out, encoded)
		e.store.Log(tx, source, fmt.Sprintf("response ESV %02X", res.ESV()), encoded)
	}
	return out, nil
}

// Notifications use the upstream serializer; broadcasts target the node profile.
func (e *Engine) Notification(change model.Change) []byte {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.tid = (e.tid + 1) & 0xffff
	msg := protocol.NewMessage()
	_ = msg.SetTID(e.tid)
	msg.SetSEOJ(protocol.ObjectCode(change.EOJ))
	msg.SetDEOJ(protocol.ObjectCode(model.Node))
	msg.SetESV(protocol.ESVNotification)
	p := protocol.NewPropertyWithCode(protocol.PropertyCode(change.EPC))
	p.SetData(change.Data)
	msg.AddProperty(p)
	return msg.Bytes()
}
func (e *Engine) Store() *model.Store { return e.store }

// Request executes the same frames as UDP without opening a socket.
func (e *Engine) Request(eoj uint32, epc byte, data []byte, write bool, source string) ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.tid = (e.tid + 1) & 0xFFFF
	req := protocol.NewMessage()
	_ = req.SetTID(e.tid)
	req.SetSEOJ(0x05FF01)
	req.SetDEOJ(protocol.ObjectCode(eoj))
	req.SetESV(protocol.ESVReadRequest)
	if write {
		req.SetESV(protocol.ESVWriteRequestResponseRequired)
	}
	p := protocol.NewPropertyWithCode(protocol.PropertyCode(epc))
	p.SetData(data)
	req.AddProperty(p)
	responses, err := e.handleAll(req.Bytes(), source, true)
	if err != nil {
		return nil, err
	}
	if len(responses) == 0 {
		return nil, fmt.Errorf("unknown device")
	}
	b := responses[0]
	res, err := protocol.NewMessageWithBytes(b)
	if err != nil {
		return nil, err
	}
	if res.ESV() == protocol.ESVReadRequestError || res.ESV() == protocol.ESVWriteRequestResponseRequiredError {
		return nil, fmt.Errorf("EPC %02X rejected (ESV %02X)", epc, byte(res.ESV()))
	}
	if !bytes.Equal(res.Bytes(), b) {
		return nil, fmt.Errorf("invalid response")
	}
	return res.Property(0).Data(), nil
}
