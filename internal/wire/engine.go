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

// validateFrame rejects truncation/trailing bytes before passing data to the
// library parser. Only Get and SetC are implemented by this prototype.
func validateFrame(b []byte) error {
	if len(b) < 12 || len(b) > 1024 || b[0] != 0x10 || b[1] != 0x81 || b[11] == 0 {
		return fmt.Errorf("invalid Format 1 frame")
	}
	if b[10] != 0x61 && b[10] != 0x62 {
		return fmt.Errorf("only Get (62) and SetC (61) are implemented")
	}
	pos := 12
	for i := 0; i < int(b[11]); i++ {
		if pos+2 > len(b) {
			return fmt.Errorf("truncated property")
		}
		n := int(b[pos+1])
		pos += 2 + n
		if pos > len(b) {
			return fmt.Errorf("truncated EDT")
		}
	}
	if pos != len(b) {
		return fmt.Errorf("trailing frame bytes")
	}
	return nil
}

func (e *Engine) Handle(b []byte, source string) ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.handle(b, source)
}
func (e *Engine) handle(b []byte, source string) ([]byte, error) {
	e.store.Log("RX", source, "request", b)
	if err := validateFrame(b); err != nil {
		e.store.Log("ERROR", source, err.Error(), nil)
		return nil, err
	}
	req, err := protocol.NewMessageWithBytes(b)
	if err != nil {
		return nil, err
	}
	if req.DEOJ() != protocol.ObjectCode(model.Light) && req.DEOJ() != protocol.ObjectCode(model.Aircon) && req.DEOJ() != protocol.ObjectCode(model.Sensor) {
		return nil, nil
	}
	res := protocol.NewResponseMessageWithMessage(req)
	failed := false
	for _, p := range req.Properties() {
		answer := protocol.NewPropertyWithCode(p.Code())
		if req.ESV() == protocol.ESVReadRequest {
			data, readErr := e.store.Read(uint32(req.DEOJ()), byte(p.Code()))
			if p.Size() != 0 {
				readErr = fmt.Errorf("Get requires empty EDT")
			}
			if readErr != nil {
				failed = true
				e.store.Log("ERROR", source, readErr.Error(), nil)
			} else {
				answer.SetData(data)
			}
		} else {
			if writeErr := e.store.Write(uint32(req.DEOJ()), byte(p.Code()), p.Data(), source); writeErr != nil {
				failed = true
				answer.SetData(p.Data())
				e.store.Log("ERROR", source, writeErr.Error(), nil)
			}
		}
		res.AddProperty(answer)
	}
	if failed {
		if req.ESV() == protocol.ESVReadRequest {
			res.SetESV(protocol.ESVReadRequestError)
		} else {
			res.SetESV(protocol.ESVWriteRequestResponseRequiredError)
		}
	}
	out := res.Bytes()
	e.store.Log("TX", source, fmt.Sprintf("response ESV %02X", byte(res.ESV())), out)
	return out, nil
}

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
	b, err := e.handle(req.Bytes(), source)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, fmt.Errorf("unknown device")
	}
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
