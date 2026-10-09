package wire

import (
	"fmt"

	"github.com/cybergarage/uecho-go/net/echonet/protocol"
)

const MaxFrameSize = 1024

// blockEnd checks the OPC/PDC structure before invoking the upstream parser.
func blockEnd(b []byte, pos int, allowZero bool) (int, error) {
	if pos >= len(b) {
		return 0, fmt.Errorf("missing OPC")
	}
	count := int(b[pos])
	pos++
	if count == 0 && !allowZero {
		return 0, fmt.Errorf("zero OPC")
	}
	for i := 0; i < count; i++ {
		if pos+2 > len(b) {
			return 0, fmt.Errorf("truncated property")
		}
		pos += 2 + int(b[pos+1])
		if pos > len(b) {
			return 0, fmt.Errorf("truncated EDT")
		}
	}
	return pos, nil
}
func validateFrame(b []byte) error {
	if len(b) < 12 || len(b) > MaxFrameSize || b[0] != 0x10 || b[1] != 0x81 {
		return fmt.Errorf("invalid Format 1 frame")
	}
	esv := protocol.ESV(b[10])
	if !esv.IsValid() {
		return fmt.Errorf("unsupported ESV")
	}
	pos, err := blockEnd(b, 11, esv == protocol.ESVWriteReadRequestError)
	if err != nil {
		return err
	}
	if esv == protocol.ESVWriteReadRequest || esv == protocol.ESVWriteReadResponse || esv == protocol.ESVWriteReadRequestError {
		pos, err = blockEnd(b, pos, esv == protocol.ESVWriteReadRequestError)
		if err != nil {
			return err
		}
	}
	if pos != len(b) {
		return fmt.Errorf("trailing frame bytes")
	}
	return nil
}

// SetGet is two upstream-coded property blocks. The pinned Message API has one
// block; this adapter preserves that codec for both blocks and all headers.
func parseRequest(b []byte) (*protocol.Message, *protocol.Message, error) {
	if err := validateFrame(b); err != nil {
		return nil, nil, err
	}
	end, _ := blockEnd(b, 11, b[10] == 0x5e)
	first, err := protocol.NewMessageWithBytes(b[:end])
	if err != nil {
		return nil, nil, err
	}
	if b[10] != 0x6e && b[10] != 0x7e && b[10] != 0x5e {
		return first, nil, nil
	}
	secondBytes := append(append([]byte{}, b[:11]...), b[end:]...)
	second, err := protocol.NewMessageWithBytes(secondBytes)
	return first, second, err
}
func combinedBytes(first, second *protocol.Message) []byte {
	out := first.Bytes()
	if second != nil {
		out = append(out, second.Bytes()[11:]...)
	}
	return out
}
