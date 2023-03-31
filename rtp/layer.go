package rtp

import (
	"encoding/binary"
	"fmt"
	"gitee.com/sy_183/common/assert"
	"gitee.com/sy_183/common/errors"
	"gitee.com/sy_183/common/slice"
	"gitee.com/sy_183/common/uns"
	ioutil "gitee.com/sy_183/common/utils/io"
	"gitee.com/sy_183/rtp/utils"
	"io"
	"math"
	"strconv"
	"strings"
)

var (
	InvalidVersionError       = errors.New("RTP版本号必须为2")
	CSRCLengthOutOfRange      = errors.New("RTP CSRC长度超过限制(0, 15)")
	ExtensionLengthOutOfRange = errors.New("RTP扩展头部长度超过限制(0, 65535)")
	PayloadLengthOutOfRange   = errors.New("RTP负载长度超过限制(0, 65535)")
	ZeroPaddingLengthError    = errors.New("RTP填充长度不能为0")
	PacketSizeNotEnoughError  = errors.New("RTP包大小不够")
)

const RtpHeaderMinSize = 12

type Payload interface {
	Size() int

	Bytes() []byte

	io.Reader

	io.WriterTo

	Clear()
}

type IncomingPayload struct {
	content []byte
	extends [][]byte
	size    int
}

func NewIncomingPayload() *IncomingPayload {
	return new(IncomingPayload)
}

func ProvideIncomingPayload() Payload {
	return NewIncomingPayload()
}

func (p *IncomingPayload) Content() []byte {
	return p.content
}

func (p *IncomingPayload) Extends() [][]byte {
	return p.extends
}

func (p *IncomingPayload) Range(f func(chunk []byte) bool) bool {
	if p.content != nil {
		if !f(p.content) {
			return false
		}
	}
	for _, chunk := range p.extends {
		if !f(chunk) {
			return false
		}
	}
	return true
}

func (p *IncomingPayload) SetContent(content []byte) *IncomingPayload {
	p.content = content
	p.extends = p.extends[:0]
	p.size = len(content)
	return p
}

func (p *IncomingPayload) setContents(contents [][]byte) {
	p.content = contents[0]
	p.extends = append(p.extends[:0], contents[1:]...)
	p.size = 0
	for _, content := range contents {
		p.size += len(content)
	}
}

func (p *IncomingPayload) SetContents(contents [][]byte) *IncomingPayload {
	if len(contents) == 0 {
		return p.SetContent(nil)
	}
	p.setContents(contents)
	return p
}

func (p *IncomingPayload) AddContent(content []byte) *IncomingPayload {
	if p.content == nil && len(p.extends) == 0 {
		p.content = content
	} else {
		p.extends = append(p.extends, content)
	}
	p.size += len(content)
	return p
}

func (p *IncomingPayload) AddContents(contents ...[]byte) *IncomingPayload {
	if len(contents) == 0 {
		return p
	}
	if p.content == nil && len(p.extends) == 0 {
		p.setContents(contents)
	} else {
		p.extends = append(p.extends, contents...)
		for _, content := range contents {
			p.size += len(content)
		}
	}
	return p
}

func (p *IncomingPayload) Size() int {
	return p.size
}

func (p *IncomingPayload) Bytes() []byte {
	w := utils.Writer{Buf: make([]byte, p.Size())}
	n, _ := p.WriteTo(&w)
	return w.Buf[:n]
}

func (p *IncomingPayload) Read(buf []byte) (n int, err error) {
	w := utils.Writer{Buf: buf}
	_n, _ := p.WriteTo(&w)
	return int(_n), io.EOF
}

func (p *IncomingPayload) WriteTo(w io.Writer) (n int64, err error) {
	// write payload content
	if err = ioutil.Write(w, p.content, &n); err != nil {
		return
	}
	// write extend payload content
	for _, content := range p.extends {
		if err = ioutil.Write(w, content, &n); err != nil {
			return
		}
	}
	return
}

func (p *IncomingPayload) Clear() {
	p.content = nil
	p.extends = p.extends[:0]
	p.size = 0
}

type Layer interface {
	Version() uint8

	HasPadding() bool

	HasExtension() bool

	CSRCCount() uint8

	Marker() bool

	SetMarker(maker bool)

	PayloadType() uint8

	SetPayloadType(payloadType uint8)

	SequenceNumber() uint16

	SetSequenceNumber(seq uint16)

	Timestamp() uint32

	SetTimestamp(timestamp uint32)

	SSRC() uint32

	SetSSRC(ssrc uint32)

	CSRCList() []uint32

	SetCSRCList(csrcList []uint32)

	AddCSRC(csrc uint32)

	AddCSRCList(csrcList ...uint32)

	ExtensionProfile() uint16

	SetExtensionProfile(profile uint16)

	ExtensionLength() uint16

	ExtensionContents() []uint32

	SetExtensionContents(contents []uint32)

	AddExtensionContent(content uint32)

	AddExtensionContents(contents ...uint32)

	PaddingLength() uint8

	SetPaddingLength(length uint8)

	DumpHeader(header *Header)

	Payload() Payload

	Size() int

	Bytes() []byte

	io.Reader

	io.WriterTo

	fmt.Stringer
}

type Header struct {
	first             uint8
	second            uint8
	sequenceNumber    uint16
	timestamp         uint32
	ssrc              uint32
	csrcList          []uint32
	extensionProfile  uint16
	extensionContents []uint32
	paddingLength     uint8
}

func (l *Header) Version() uint8 {
	return l.first >> 6
}

func (l *Header) HasPadding() bool {
	return l.first&0b00100000 != 0
}

func (l *Header) HasExtension() bool {
	return l.first&0b00010000 != 0
}

func (l *Header) CSRCCount() uint8 {
	return uint8(len(l.csrcList))
}

func (l *Header) Marker() bool {
	return l.second&0b10000000 != 0
}

func (l *Header) SetMarker(maker bool) {
	if maker {
		l.second |= 0b10000000
	} else {
		l.second &= ^uint8(0b10000000)
	}
}

func (l *Header) PayloadType() uint8 {
	return l.second & 0b01111111
}

func (l *Header) SetPayloadType(payloadType uint8) {
	l.second |= payloadType & 0b01111111
}

func (l *Header) SequenceNumber() uint16 {
	return l.sequenceNumber
}

func (l *Header) SetSequenceNumber(seq uint16) {
	l.sequenceNumber = seq
}

func (l *Header) Timestamp() uint32 {
	return l.timestamp
}

func (l *Header) SetTimestamp(timestamp uint32) {
	l.timestamp = timestamp
}

func (l *Header) SSRC() uint32 {
	return l.ssrc
}

func (l *Header) SetSSRC(ssrc uint32) {
	l.ssrc = ssrc
}

func (l *Header) CSRCList() []uint32 {
	return l.csrcList
}

func (l *Header) SetCSRCList(csrcList []uint32) {
	if len(csrcList) > 15 {
		csrcList = csrcList[:15]
	}
	l.csrcList = csrcList
}

func (l *Header) AddCSRC(csrc uint32) {
	if len(l.csrcList) < 15 {
		l.csrcList = append(l.csrcList, csrc)
	}
}

func (l *Header) AddCSRCList(csrcList ...uint32) {
	limit := 15 - len(l.csrcList)
	if len(csrcList) > limit {
		csrcList = csrcList[:limit]
	}
	l.csrcList = append(l.csrcList, csrcList...)
}

func (l *Header) ExtensionProfile() uint16 {
	return l.extensionProfile
}

func (l *Header) SetExtensionProfile(profile uint16) {
	l.extensionProfile = profile
}

func (l *Header) ExtensionLength() uint16 {
	return uint16(len(l.extensionContents))
}

func (l *Header) ExtensionContents() []uint32 {
	return l.extensionContents
}

func (l *Header) SetExtensionContents(contents []uint32) {
	if len(contents) > math.MaxUint16 {
		contents = contents[:15]
	}
	l.extensionContents = contents
}

func (l *Header) AddExtensionContent(content uint32) {
	if len(l.extensionContents) < math.MaxUint16 {
		l.extensionContents = append(l.extensionContents, content)
	}
}

func (l *Header) AddExtensionContents(contents ...uint32) {
	limit := math.MaxUint16 - len(l.extensionContents)
	if len(contents) > limit {
		contents = contents[:limit]
	}
	l.extensionContents = append(l.extensionContents, contents...)
}

func (l *Header) PaddingLength() uint8 {
	return l.paddingLength
}

func (l *Header) SetPaddingLength(length uint8) {
	l.paddingLength = length
	l.first |= 0b00100000
}

func (l *Header) DumpHeader(header *Header) {
	*header = *l
}

func (l *Header) Clear() {
	l.first = 2 << 6
	l.second = 0
	l.sequenceNumber = 0
	l.timestamp = 0
	l.ssrc = 0
	l.csrcList = l.csrcList[:0]
	l.extensionProfile = 0
	l.extensionContents = l.extensionContents[:0]
	l.paddingLength = 0
}

type combinedLayer struct {
	*Header
	payload Payload
}

func (l combinedLayer) Payload() Payload {
	return l.payload
}

func (l combinedLayer) Size() int {
	var extensionSize, paddingLength int
	if l.HasExtension() {
		extensionSize = 4 + int(l.ExtensionLength())*4
	}
	if l.HasPadding() {
		paddingLength = int(l.PaddingLength())
	}
	return 12 + int(l.CSRCCount())*4 + extensionSize + l.payload.Size() + paddingLength
}

func (l combinedLayer) Bytes() []byte {
	w := utils.Writer{Buf: make([]byte, l.Size())}
	n, _ := l.WriteTo(&w)
	return w.Buf[:n]
}

func (l combinedLayer) Read(p []byte) (n int, err error) {
	w := utils.Writer{Buf: p}
	_n, _ := l.WriteTo(&w)
	return int(_n), io.EOF
}

const writeBufLen = 16

func (l combinedLayer) WriteTo(w io.Writer) (n int64, err error) {
	var buf [writeBufLen]byte
	writer := ioutil.Writer{Buf: buf[:]}

	defer func() { err = ioutil.HandleRecovery(recover()) }()

	// write base header
	buf[0], buf[1] = (l.CSRCCount()&0b00001111)|(l.first&0b11110000), l.second
	binary.BigEndian.PutUint16(buf[2:], l.sequenceNumber)
	binary.BigEndian.PutUint32(buf[4:], l.timestamp)
	binary.BigEndian.PutUint32(buf[8:], l.ssrc)
	ioutil.WritePanic(w, buf[:12], &n)

	// write csrc list
	for _, csrc := range l.csrcList {
		if writer.WriteUint32(csrc); writer.Len() == writeBufLen {
			ioutil.WriteAndResetPanic(&writer, w, &n)
		}
	}
	ioutil.WriteAndResetPanic(&writer, w, &n)

	// write extension if necessary
	if l.HasExtension() {
		// write extension header
		extensionLength := l.ExtensionLength()
		writer.WriteUint16(l.extensionProfile).WriteUint16(extensionLength)

		// write extension content
		for i := uint16(0); i < extensionLength; i++ {
			if writer.WriteUint32(l.extensionContents[i]); writer.Len() == writeBufLen {
				ioutil.WriteAndResetPanic(&writer, w, &n)
			}
		}
		ioutil.WriteAndResetPanic(&writer, w, &n)
	}

	// write payload content
	ioutil.WriteToPanic(l.payload, w, &n)

	// write padding
	if l.HasPadding() && l.paddingLength > 0 {
		*uns.ConvertPointer[byte, uint64](&buf[0]) = 0
		*uns.ConvertPointer[byte, uint64](&buf[8]) = 0
		for remain := l.paddingLength; true; remain -= uint8(writeBufLen) {
			if remain <= uint8(writeBufLen) {
				buf[remain-1] = l.paddingLength
				ioutil.WritePanic(w, buf[:remain], &n)
				break
			}
			ioutil.WritePanic(w, buf[:], &n)
		}
	}

	return
}

func (l combinedLayer) String() string {
	sb := strings.Builder{}
	sb.WriteString("RTP(Length=")
	sb.WriteString(strconv.FormatUint(uint64(l.Size()), 10))
	sb.WriteString(", PT=")
	sb.WriteString(strconv.FormatUint(uint64(l.PayloadType()), 10))
	sb.WriteString(", SSRC=0x")
	sb.WriteString(strconv.FormatUint(uint64(l.ssrc), 16))
	if csrcCount := len(l.csrcList); csrcCount > 0 {
		csrcList := l.csrcList
		sb.WriteString(", CSRC=[")
		for i := 0; i < csrcCount; i++ {
			sb.WriteString("0x")
			sb.WriteString(strconv.FormatUint(uint64(csrcList[i]), 16))
			if i != csrcCount-1 {
				sb.WriteString(", ")
			}
		}
		sb.WriteString("]")
	}
	sb.WriteString(", Seq=")
	sb.WriteString(strconv.FormatUint(uint64(l.sequenceNumber), 10))
	sb.WriteString(", Time=")
	sb.WriteString(strconv.FormatUint(uint64(l.timestamp), 10))
	sb.WriteString(", Payload=")
	sb.WriteString(strconv.FormatUint(uint64(l.payload.Size()), 10))
	if l.HasPadding() {
		sb.WriteString(", Padding")
	}
	if l.HasExtension() {
		sb.WriteString(", Extent")
	}
	if l.Marker() {
		sb.WriteString(", Mark")
	}
	sb.WriteString(")")
	return sb.String()
}

type IncomingLayer struct {
	Header
	payload IncomingPayload
}

type IncomingLayerConfig struct {
	Marker            bool
	PayloadType       uint8
	SequenceNumber    uint16
	Timestamp         uint32
	SSRC              uint32
	CSRCList          []uint32
	ExtensionProfile  uint16
	ExtensionContents []uint32
	PaddingLength     uint8
	PayloadContent    []byte
	PayloadContents   [][]byte
}

func NewIncomingLayer() *IncomingLayer {
	return &IncomingLayer{Header: Header{first: 2 << 6}}
}

func ProvideIncomingLayer() Layer {
	return NewIncomingLayer()
}

func BuildIncomingLayer(cfg *IncomingLayerConfig) *IncomingLayer {
	assert.PtrNotNil(cfg, "layer config")
	l := &IncomingLayer{
		Header: Header{
			sequenceNumber:    cfg.SequenceNumber,
			timestamp:         cfg.Timestamp,
			ssrc:              cfg.SSRC,
			csrcList:          cfg.CSRCList,
			extensionProfile:  cfg.ExtensionProfile,
			extensionContents: cfg.ExtensionContents,
			paddingLength:     cfg.PaddingLength,
		},
		payload: IncomingPayload{
			content: cfg.PayloadContent,
			extends: cfg.PayloadContents,
		},
	}
	l.first = (2 << 6) | // version
		(uint8(len(cfg.CSRCList)) & 0b00001111) // CSRC count
	if l.paddingLength > 0 {
		l.first |= 0b00100000
	}
	if len(l.extensionContents) > 0 {
		l.first |= 0b00010000
	}
	l.second = cfg.PayloadType & 0b01111111
	if cfg.Marker {
		l.second |= 0b10000000
	}
	return l
}

func (l *IncomingLayer) Payload() Payload {
	return &l.payload
}

func (l *IncomingLayer) Size() int {
	return combinedLayer{Header: &l.Header, payload: &l.payload}.Size()
}

func (l *IncomingLayer) parseBaseHeader(baseHeader []byte, length int, minSize *int) (uint8, error) {
	l.first = baseHeader[0]
	l.second = baseHeader[1]
	if version := l.Version(); version != 2 {
		return 0, InvalidVersionError
	}
	l.sequenceNumber = binary.BigEndian.Uint16(baseHeader[2:4])
	l.timestamp = binary.BigEndian.Uint32(baseHeader[4:8])
	l.ssrc = binary.BigEndian.Uint32(baseHeader[8:12])
	csrcCount := l.first & 0b00001111

	// ensure packet size
	*minSize += int(csrcCount) * 4
	if l.HasExtension() {
		*minSize += 4
	}
	if l.HasPadding() {
		*minSize += 1
	}
	if length < *minSize {
		return csrcCount, PacketSizeNotEnoughError
	}
	return csrcCount, nil
}

func (l *IncomingLayer) parseCSRC(csrcData []byte, csrcCount uint8) {
	l.csrcList = slice.AssignLen(l.csrcList[:0], int(csrcCount))
	for i := uint8(0); i < csrcCount; i++ {
		l.csrcList[i] = binary.BigEndian.Uint32(csrcData[:4])
		csrcData = csrcData[4:]
	}
	return
}

func (l *IncomingLayer) parseExtensionHeader(extensionHeader []byte, length int, minSize *int) (uint16, error) {
	l.extensionProfile = binary.BigEndian.Uint16(extensionHeader[:2])
	extensionLength := binary.BigEndian.Uint16(extensionHeader[2:4])

	// ensure packet size
	*minSize += int(extensionLength) * 4
	if length < *minSize {
		return extensionLength, PacketSizeNotEnoughError
	}
	return extensionLength, nil
}

func (l *IncomingLayer) parseExtensionData(extensionData []byte, extensionLength uint16) {
	l.extensionContents = slice.AssignLen(l.extensionContents[:0], int(extensionLength))
	for i := uint16(0); i < extensionLength; i++ {
		l.extensionContents[i] = binary.BigEndian.Uint32(extensionData[:4])
		extensionData = extensionData[4:]
	}
}

func (l *IncomingLayer) parsePadding(length int, minSize *int) error {
	if l.paddingLength == 0 {
		return ZeroPaddingLengthError
	}

	// ensure packet size
	*minSize += int(l.paddingLength - 1)
	if length < *minSize {
		return PacketSizeNotEnoughError
	}
	return nil
}

func (l *IncomingLayer) Unmarshal(data []byte) error {
	minSize := RtpHeaderMinSize
	length := len(data)
	if length < minSize {
		return PacketSizeNotEnoughError
	}

	// parse base header
	baseHeader, cur := data[:12], data[12:]
	csrcCount, err := l.parseBaseHeader(baseHeader, length, &minSize)
	if err != nil {
		return err
	}

	// parse csrc list
	csrcData, cur := cur[:csrcCount*4], cur[csrcCount*4:]
	l.parseCSRC(csrcData, csrcCount)

	// parse extension if exist
	if l.HasExtension() {
		// parse extension header
		var extensionHeader, extensionData []byte
		extensionHeader, cur = cur[:4], cur[4:]
		extensionLength, err := l.parseExtensionHeader(extensionHeader, length, &minSize)
		if err != nil {
			return err
		}

		// parse extension content
		extensionData, cur = cur[:extensionLength*4], cur[extensionLength*4:]
		l.parseExtensionData(extensionData, extensionLength)
	}

	// parse padding
	if l.HasPadding() {
		l.paddingLength = cur[len(cur)-1]
		if err := l.parsePadding(length, &minSize); err != nil {
			return err
		}
	}

	// parse payload content
	payload := &l.payload
	payload.size = length - minSize
	payload.content = cur[:payload.size]
	payload.extends = payload.extends[:0]
	return nil
}

func (l *IncomingLayer) UnmarshalChunks(cs [][]byte) error {
	if len(cs) == 1 {
		return l.Unmarshal(cs[0])
	}
	var length int
	var minSize = RtpHeaderMinSize
	for _, chunk := range cs {
		length += len(chunk)
	}
	if length < minSize {
		return PacketSizeNotEnoughError
	}

	chunks := slice.Chunks[byte](cs)
	baseHeader, cur := chunks.Slice(-1, 12), chunks.Cut(12, -1)
	csrcCount, err := l.parseBaseHeader(baseHeader, length, &minSize)
	if err != nil {
		return err
	}

	// parse csrc list
	csrcDataSize := int(csrcCount * 4)
	csrcData, cur := cur.Slice(-1, csrcDataSize), cur.Cut(csrcDataSize, -1)
	l.parseCSRC(csrcData, csrcCount)

	if l.HasExtension() {
		// parse extension header
		var extensionHeader, extensionData []byte
		extensionHeader, cur = cur.Slice(-1, 4), cur.Cut(4, -1)
		extensionLength, err := l.parseExtensionHeader(extensionHeader, length, &minSize)
		if err != nil {
			return err
		}

		// parse extension content
		extensionDataSize := int(extensionLength * 4)
		extensionData, cur = cur.Slice(-1, extensionDataSize), cur.Cut(extensionDataSize, -1)
		l.parseExtensionData(extensionData, extensionLength)
	}

	// parse padding
	if l.HasPadding() {
		l.paddingLength, _ = chunks.Last()
		if err := l.parsePadding(length, &minSize); err != nil {
			return err
		}
	}

	// parse payload content
	payloadSize := length - minSize

	payload := &l.payload
	if payloadSize > 0 {
		contents := cur.Cut(-1, payloadSize)
		payload.size = payloadSize
		payload.content = contents[0]
		payload.extends = append(payload.extends[:0], contents[1:]...)
	} else {
		payload.size = 0
		payload.content = nil
		payload.extends = payload.extends[:0]
	}

	return nil
}

func (l *IncomingLayer) Read(p []byte) (n int, err error) {
	return combinedLayer{Header: &l.Header, payload: &l.payload}.Read(p)
}

func (l *IncomingLayer) Bytes() []byte {
	return combinedLayer{Header: &l.Header, payload: &l.payload}.Bytes()
}

func (l *IncomingLayer) WriteTo(w io.Writer) (n int64, err error) {
	return combinedLayer{Header: &l.Header, payload: &l.payload}.WriteTo(w)
}

func (l *IncomingLayer) String() string {
	return combinedLayer{Header: &l.Header, payload: &l.payload}.String()
}

type DefaultLayer struct {
	Header
	payload Payload
}

func NewDefaultLayer(payload Payload) *DefaultLayer {
	return &DefaultLayer{Header: Header{first: 2 << 6}, payload: payload}
}

func DefaultLayerProvider(payloadProvider func() Payload) func() Layer {
	return func() Layer {
		return NewDefaultLayer(payloadProvider())
	}
}

func (l *DefaultLayer) Init(payload Payload) {
	l.first = 2 << 6
	l.payload = payload
}

func (l *DefaultLayer) Payload() Payload {
	return l.payload
}

func (l *DefaultLayer) SetPayload(payload Payload) {
	l.payload = payload
}

func (l *DefaultLayer) Size() int {
	return combinedLayer{Header: &l.Header, payload: l.payload}.Size()
}

func (l *DefaultLayer) Bytes() []byte {
	return combinedLayer{Header: &l.Header, payload: l.payload}.Bytes()
}

func (l *DefaultLayer) Read(p []byte) (n int, err error) {
	return combinedLayer{Header: &l.Header, payload: l.payload}.Read(p)
}

func (l *DefaultLayer) WriteTo(w io.Writer) (n int64, err error) {
	return combinedLayer{Header: &l.Header, payload: l.payload}.WriteTo(w)
}

func (l *DefaultLayer) String() string {
	return combinedLayer{Header: &l.Header, payload: l.payload}.String()
}
