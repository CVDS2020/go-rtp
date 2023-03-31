package frame

import (
	"bytes"
	"encoding/binary"
	"gitee.com/sy_183/common/pool"
	"gitee.com/sy_183/rtp/rtp"
	"gitee.com/sy_183/rtp/utils"
	"io"
	"net"
	"time"
)

type FrameWriter interface {
	Frame() Frame

	SetFrame(frame Frame)

	Size() int

	WriteTo(w io.Writer) (n int64, err error)

	Read(p []byte) (n int, err error)
}

type abstractFrameWriter struct {
	frame Frame
	size  int
}

func (f *abstractFrameWriter) Frame() Frame {
	return f.frame
}

func (f *abstractFrameWriter) SetFrame(frame Frame) {
	f.frame = frame
	f.size = 0
}

type FullFrameWriter struct {
	abstractFrameWriter
	packet rtp.DefaultPacket

	timestamp       uint32
	timestampEnable bool

	payloadType       uint8
	payloadTypeEnable bool

	sequenceNumber       uint16
	sequenceNumberEnable bool

	ssrc       uint32
	ssrcEnable bool
}

func NewFullFrameWriter(frame Frame) *FullFrameWriter {
	return &FullFrameWriter{abstractFrameWriter: abstractFrameWriter{frame: frame}}
}

func (f *FullFrameWriter) Timestamp() (uint32, bool) {
	if f.timestampEnable {
		return f.timestamp, true
	}
	return 0, false
}

func (f *FullFrameWriter) PayloadType() (uint8, bool) {
	if f.payloadTypeEnable {
		return f.payloadType, true
	}
	return 0, false
}

func (f *FullFrameWriter) SequenceNumber() (uint16, bool) {
	if f.sequenceNumberEnable {
		return f.sequenceNumber, true
	}
	return 0, false
}

func (f *FullFrameWriter) SSRC() (uint32, bool) {
	if f.ssrcEnable {
		return f.ssrc, true
	}
	return 0, false
}

func (f *FullFrameWriter) SetTimestamp(timestamp uint32) {
	f.timestamp = timestamp
	f.timestampEnable = true
}

func (f *FullFrameWriter) SetPayloadType(payloadType uint8) {
	if payloadType < 128 {
		f.payloadType = payloadType
		f.payloadTypeEnable = true
	}
}

func (f *FullFrameWriter) SetSequenceNumber(sequenceNumber uint16) {
	f.sequenceNumber = sequenceNumber
	f.sequenceNumberEnable = true
}

func (f *FullFrameWriter) SetSSRC(ssrc uint32) {
	f.ssrc = ssrc
	f.ssrcEnable = true
}

func (f *FullFrameWriter) DisableTimestamp() {
	f.timestamp = 0
	f.timestampEnable = false
}

func (f *FullFrameWriter) DisablePayloadType() {
	f.payloadType = 0
	f.payloadTypeEnable = false
}

func (f *FullFrameWriter) DisableSequenceNumber() {
	f.sequenceNumber = 0
	f.sequenceNumberEnable = false
}

func (f *FullFrameWriter) DisableSSRC() {
	f.ssrc = 0
	f.ssrcEnable = false
}

func (f *FullFrameWriter) Size() int {
	if f.size != 0 {
		return f.size
	}
	var s int
	f.frame.Range(func(i int, packet rtp.Packet) bool {
		s += packet.Size() + 2
		return true
	})
	f.size = s
	return s
}

func (f *FullFrameWriter) detachPacket(packetP *rtp.Packet, detached *bool) {
	if !*detached {
		(*packetP).DumpHeader(&f.packet.Header)
		f.packet.SetPayload((*packetP).Payload())
		*detached = true
		*packetP = &f.packet
	}
}

func (f *FullFrameWriter) modifyPacket(packetP *rtp.Packet) (modified bool) {
	packet := *packetP
	var detached bool
	if timestamp, ok := f.Timestamp(); ok {
		f.detachPacket(&packet, &detached)
		packet.SetTimestamp(timestamp)
	}
	if payloadType, ok := f.PayloadType(); ok {
		f.detachPacket(&packet, &detached)
		packet.SetPayloadType(payloadType)
	}
	if sequenceNumber, ok := f.SequenceNumber(); ok {
		f.detachPacket(&packet, &detached)
		packet.SetSequenceNumber(sequenceNumber)
		f.sequenceNumber++
	}
	if ssrc, ok := f.SSRC(); ok {
		f.detachPacket(&packet, &detached)
		packet.SetSSRC(ssrc)
	}
	*packetP = packet
	return detached
}

func (f *FullFrameWriter) WriteTo(w io.Writer) (n int64, err error) {
	var buf [2]byte
	f.frame.Range(func(i int, packet rtp.Packet) bool {
		f.modifyPacket(&packet)
		binary.BigEndian.PutUint16(buf[:], uint16(packet.Size()))
		if err = utils.Write(w, buf[:], &n); err != nil {
			return false
		}
		if err = utils.WriteTo(packet, w, &n); err != nil {
			return false
		}
		return true
	})
	return
}

func (f *FullFrameWriter) WriteToUDP(buffer *bytes.Buffer, udpConn *net.UDPConn) (err error) {
	f.frame.Range(func(i int, packet rtp.Packet) bool {
		defer buffer.Reset()
		f.modifyPacket(&packet)
		packet.WriteTo(buffer)
		if _, err = udpConn.Write(buffer.Bytes()); err != nil {
			return false
		}
		return true
	})
	return
}

func (f *FullFrameWriter) Read(p []byte) (n int, err error) {
	w := utils.Writer{Buf: p}
	n64, _ := f.WriteTo(&w)
	return int(n64), io.EOF
}

type PayloadFrameWriter struct {
	abstractFrameWriter
}

func NewPayloadFrameWriter(frame Frame) FrameWriter {
	return &PayloadFrameWriter{abstractFrameWriter{frame: frame}}
}

func (f *PayloadFrameWriter) Size() int {
	if f.size != 0 {
		return f.size
	}
	var s int
	f.frame.Range(func(i int, packet rtp.Packet) bool {
		s += packet.Payload().Size()
		return true
	})
	f.size = s
	return s
}

func (f *PayloadFrameWriter) WriteTo(w io.Writer) (n int64, err error) {
	f.frame.Range(func(i int, packet rtp.Packet) bool {
		if err = utils.WriteTo(packet.Payload(), w, &n); err != nil {
			return false
		}
		return true
	})
	return
}

func (f *PayloadFrameWriter) Read(p []byte) (n int, err error) {
	w := utils.Writer{Buf: p}
	n64, _ := f.WriteTo(&w)
	return int(n64), io.EOF
}

type Frame interface {
	Len() int

	Range(f func(i int, packet rtp.Packet) bool) bool

	Packet(i int) rtp.Packet

	Timestamp() uint32

	SetTimestamp(timestamp uint32)

	PayloadType() uint8

	SetPayloadType(typ uint8)

	Append(packet rtp.Packet) bool

	AddRelation(relation pool.Reference)

	Clear()

	pool.Reference
}

func UseFrame(frame Frame) Frame {
	frame.AddRef()
	return frame
}

type IncomingFrame struct {
	packets     []*rtp.IncomingPacket
	timestamp   uint32
	payloadType uint8
	start       time.Time
	end         time.Time
	relations   pool.Relations
	pool        pool.Pool[*IncomingFrame]
}

func NewFrame() Frame {
	return new(IncomingFrame)
}

func timeRangeExpand(start, end *time.Time, cur time.Time, timeNodeLen int) {
	if start.After(*end) {
		panic("start time after end time")
	}
	if start.IsZero() {
		*start = cur
		*end = cur
	} else {
		du := cur.Sub(*end)
		if du < 0 {
			guess := time.Duration(0)
			if timeNodeLen > 2 {
				guess = (end.Sub(*start)) / time.Duration(timeNodeLen-2)
			}
			offset := du - guess
			*start = start.Add(offset)
		}
		*end = cur
	}
}

func (f *IncomingFrame) Len() int {
	return len(f.packets)
}

func (f *IncomingFrame) Range(fn func(i int, packet rtp.Packet) bool) bool {
	for i, packet := range f.packets {
		if !fn(i, packet) {
			return false
		}
	}
	return true
}

func (f *IncomingFrame) Packet(i int) rtp.Packet {
	return f.packets[i]
}

func (f *IncomingFrame) Timestamp() uint32 {
	return f.timestamp
}

func (f *IncomingFrame) SetTimestamp(timestamp uint32) {
	for _, packet := range f.packets {
		packet.SetTimestamp(timestamp)
	}
	f.timestamp = timestamp
}

func (f *IncomingFrame) PayloadType() uint8 {
	return f.payloadType
}

func (f *IncomingFrame) SetPayloadType(typ uint8) {
	for _, packet := range f.packets {
		packet.SetPayloadType(typ)
	}
	f.payloadType = typ
}

func (f *IncomingFrame) Start() time.Time {
	return f.start
}

func (f *IncomingFrame) End() time.Time {
	return f.end
}

func (f *IncomingFrame) Append(packet rtp.Packet) bool {
	if len(f.packets) == 0 {
		f.timestamp = packet.Timestamp()
		f.payloadType = packet.PayloadType()
	} else if f.timestamp != packet.Timestamp() || f.payloadType != packet.PayloadType() {
		return false
	}
	icPacket := packet.(*rtp.IncomingPacket)
	f.packets = append(f.packets, icPacket)
	timeRangeExpand(&f.start, &f.end, icPacket.Time(), len(f.packets))
	return true
}

var zeroTime = time.Time{}

func (f *IncomingFrame) AddRelation(relation pool.Reference) {
	f.relations.AddRelation(relation)
}

func (f *IncomingFrame) Clear() {
	for _, packet := range f.packets {
		packet.Release()
	}
	f.packets = f.packets[:0]
	f.timestamp = 0
	f.payloadType = 0
	f.start, f.end = zeroTime, zeroTime
}

func (f *IncomingFrame) Release() bool {
	if f.relations.Release() {
		f.Clear()
		if f.pool != nil {
			f.pool.Put(f)
		}
		return true
	}
	return false
}

func (f *IncomingFrame) AddRef() {
	f.relations.AddRef()
}

func (f *IncomingFrame) Use() *IncomingFrame {
	f.AddRef()
	return f
}

type DefaultFrame struct {
	packets     []rtp.Packet
	timestamp   uint32
	payloadType uint8
	relations   pool.Relations
	pool        pool.Pool[*DefaultFrame]
}

func NewDefaultFrame(pool pool.Pool[*DefaultFrame]) *DefaultFrame {
	return &DefaultFrame{pool: pool}
}

func (f *DefaultFrame) Len() int {
	return len(f.packets)
}

func (f *DefaultFrame) Range(fn func(i int, packet rtp.Packet) bool) bool {
	for i, packet := range f.packets {
		if !fn(i, packet) {
			return false
		}
	}
	return true
}

func (f *DefaultFrame) Packet(i int) rtp.Packet {
	return f.packets[i]
}

func (f *DefaultFrame) Timestamp() uint32 {
	return f.timestamp
}

func (f *DefaultFrame) SetTimestamp(timestamp uint32) {
	for _, packet := range f.packets {
		packet.SetTimestamp(timestamp)
	}
	f.timestamp = timestamp
}

func (f *DefaultFrame) PayloadType() uint8 {
	return f.payloadType
}

func (f *DefaultFrame) SetPayloadType(typ uint8) {
	for _, packet := range f.packets {
		packet.SetPayloadType(typ)
	}
	f.payloadType = typ
}

func (f *DefaultFrame) Append(packet rtp.Packet) bool {
	if len(f.packets) == 0 {
		f.timestamp = packet.Timestamp()
		f.payloadType = packet.PayloadType()
	} else if f.timestamp != packet.Timestamp() || f.payloadType != packet.PayloadType() {
		return false
	}
	f.packets = append(f.packets, packet)
	return true
}

func (f *DefaultFrame) AddRelation(relation pool.Reference) {
	f.relations.AddRelation(relation)
}

func (f *DefaultFrame) Clear() {
	for _, packet := range f.packets {
		packet.Release()
	}
	f.packets = f.packets[:0]
	f.timestamp = 0
	f.payloadType = 0
}

func (f *DefaultFrame) AddRef() {
	f.relations.AddRef()
}

func (f *DefaultFrame) Release() bool {
	if f.relations.Release() {
		f.Clear()
		if f.pool != nil {
			f.pool.Put(f)
		}
		return true
	}
	return false
}

func (f *DefaultFrame) Use() *DefaultFrame {
	f.AddRef()
	return f
}
