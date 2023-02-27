package rtp

import (
	"encoding/binary"
	"errors"
)

var InvalidTCPHeaderIDError = errors.New("invalid tcp header id")

//type TCPHeader struct {
//	ID      byte
//	Channel uint8
//	Length  uint16
//}

type Parser struct {
	//ID     byte
	Layer  *Layer
	Length uint16
	//Err       error
	header []byte
	need   int
	cache  []byte
}

func (p *Parser) reset() {
	p.header = p.header[:0]
	p.need = 0
	p.cache = p.cache[:0]
}

func (p *Parser) Parse(buf []byte) (ok bool, remain []byte, err error) {
	if len(buf) == 0 {
		return
	}

	// parse header(4 bytes)
	if len(p.header) < 2 {
		//if len(p.header) == 0 {
		//	// first parse, check header ID
		//	if buf[0] != p.ID {
		//		// p.Err must be nil, because if err occurred, find correct
		//		// header ID first
		//		remain = buf[1:]
		//		p.Err = InvalidTCPHeaderIDError
		//		err = p.Err
		//		return
		//	} else if p.Err != nil {
		//		// header ID found, clear last error
		//		p.Err = nil
		//	}
		//}
		// hn: header need bytes
		hn := 2 - len(p.header)
		if len(buf) >= hn {
			// header can be parsed, tcp header and need length will be set
			p.header = append(p.header, buf[:hn]...)
			buf = buf[hn:]
			p.Length = binary.BigEndian.Uint16(p.header)
			p.need = int(p.Length)
		} else {
			// header cannot be parsed, because bytes not enough
			p.header = append(p.header, buf...)
			return
		}
	}

	if len(buf) == 0 {
		return
	}

	// parse rtp body
	if len(buf) >= p.need {
		var data []byte
		if len(p.cache) > 0 {
			p.cache = append(p.cache, buf[:p.need]...)
			data = p.cache
		} else {
			data = buf[:p.need]
		}
		if err = p.Layer.Unmarshal(data); err == nil {
			ok = true
		}
		// parse completion, reset parser
		remain = buf[p.need:]
		p.reset()
		return
	}
	// bytes not enough, add to cache
	p.cache = append(p.cache, buf...)
	p.need -= len(buf)
	return
}
