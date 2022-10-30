package rtp

import (
	"encoding/binary"
	"gitee.com/sy_183/common/assert"
	"gitee.com/sy_183/common/errors"
	"gitee.com/sy_183/common/slice"
	"gitee.com/sy_183/rtp/utils"
	"io"
	"strconv"
	"strings"
)

var (
	InvalidVersionError = errors.New("rtp version must be 2")

	CSRCLengthOutOfRange = errors.New("rtp CSRC length out of range(0, 15)")

	ExtensionLengthOutOfRange = errors.New("rtp extension length out of range(0, 65535)")

	PayloadLengthOutOfRange = errors.New("rtp payload length out of range(0, 65535)")

	ZeroPaddingLengthError = errors.New("rtp padding length must not be zero")

	PacketSizeNotEnoughError = errors.New("rtp packet size not enough")
)

const RTPHeaderMinSize = 12

type PayloadContent struct {
	content []byte
	extends [][]byte
}

func (c *PayloadContent) Content() []byte {
	return c.content
}

func (c *PayloadContent) Extends() [][]byte {
	return c.extends
}

func (c *PayloadContent) Size() uint {
	l := len(c.content)
	for _, extend := range c.extends {
		l += len(extend)
	}
	return uint(l)
}

func (c *PayloadContent) WriteTo(w io.Writer) (n int64, err error) {
	// write payload content
	if err = utils.Write(w, c.content, &n); err != nil {
		return
	}

	// write extend payload content
	for _, content := range c.extends {
		if err = utils.Write(w, content, &n); err != nil {
			return
		}
	}

	return
}

func (c *PayloadContent) Read(p []byte) (n int, err error) {
	w := utils.Writer{Buf: p}
	_n, _ := c.WriteTo(&w)
	return int(_n), io.EOF
}

func (c *PayloadContent) Bytes() []byte {
	w := utils.Writer{Buf: make([]byte, c.Size())}
	n, _ := c.WriteTo(&w)
	return w.Buf[:n]
}

type Layer struct {
	first             uint8
	second            uint8
	sequenceNumber    uint16
	timestamp         uint32
	ssrc              uint32
	csrcList          []uint32
	extensionProfile  uint16
	extensionContents []uint32
	paddingLength     uint8
	PayloadContent
}

type LayerConfig struct {
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
	payloadContents   [][]byte
}

func NewLayer() *Layer {
	return &Layer{first: 2 << 6}
}

func BuildLayer(cfg *LayerConfig) *Layer {
	assert.PtrNotNil(cfg, "layer config")
	l := &Layer{
		sequenceNumber:    cfg.SequenceNumber,
		timestamp:         cfg.Timestamp,
		ssrc:              cfg.SSRC,
		csrcList:          cfg.CSRCList,
		extensionProfile:  cfg.ExtensionProfile,
		extensionContents: cfg.ExtensionContents,
		paddingLength:     cfg.PaddingLength,
		PayloadContent: PayloadContent{
			content: cfg.PayloadContent,
			extends: cfg.payloadContents,
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

func (l *Layer) Version() uint8 {
	return l.first >> 6
}

func (l *Layer) HasPadding() bool {
	return l.first&0b00100000 != 0
}

func (l *Layer) HasExtension() bool {
	return l.first&0b00010000 != 0
}

func (l *Layer) CSRCCount() uint8 {
	return uint8(len(l.csrcList))
}

func (l *Layer) Marker() bool {
	return l.second&0b10000000 != 0
}

func (l *Layer) SetMarker(maker bool) *Layer {
	if maker {
		l.second |= 0b10000000
	} else {
		l.second &= ^uint8(0b10000000)
	}
	return l
}

func (l *Layer) PayloadType() uint8 {
	return l.second & 0b01111111
}

func (l *Layer) SetPayloadType(payloadType uint8) *Layer {
	l.second |= payloadType & 0b01111111
	return l
}

func (l *Layer) SequenceNumber() uint16 {
	return l.sequenceNumber
}

func (l *Layer) SetSequenceNumber(seq uint16) *Layer {
	l.sequenceNumber = seq
	return l
}

func (l *Layer) Timestamp() uint32 {
	return l.timestamp
}

func (l *Layer) SetTimestamp(timestamp uint32) *Layer {
	l.timestamp = timestamp
	return l
}

func (l *Layer) SSRC() uint32 {
	return l.ssrc
}

func (l *Layer) SetSSRC(ssrc uint32) *Layer {
	l.ssrc = ssrc
	return l
}

func (l *Layer) CSRCList() []uint32 {
	return l.csrcList
}

func (l *Layer) SetCSRCList(csrcList []uint32) *Layer {
	l.csrcList = csrcList
	return l
}

func (l *Layer) AddCSRC(csrc uint32) *Layer {
	l.csrcList = append(l.csrcList, csrc)
	return l
}

func (l *Layer) ExtensionProfile() uint16 {
	return l.extensionProfile
}

func (l *Layer) SetExtensionProfile(profile uint16) {
	l.extensionProfile = profile
}

func (l *Layer) ExtensionLength() uint16 {
	return uint16(len(l.extensionContents))
}

func (l *Layer) ExtensionContents() []uint32 {
	return l.extensionContents
}

func (l *Layer) SetExtensionContents(contents []uint32) *Layer {
	l.extensionContents = contents
	return l
}

func (l *Layer) AddExtensionContent(content uint32) *Layer {
	l.extensionContents = append(l.extensionContents, content)
	return l
}

func (l *Layer) PayloadLength() uint {
	return l.PayloadContent.Size()
}

func (l *Layer) PaddingLength() uint8 {
	return l.paddingLength
}

func (l *Layer) SetPaddingLength(length uint8) {
	l.paddingLength = length
	l.first |= 0b00100000
}

func (l *Layer) Size() uint {
	var extensionSize, paddingLength uint
	if l.HasExtension() {
		extensionSize = 4 + uint(l.ExtensionLength())*4
	}
	if l.HasPadding() {
		paddingLength = uint(l.paddingLength)
	}
	return 12 + uint(l.CSRCCount())*4 + extensionSize + l.PayloadContent.Size() + paddingLength
}

func (l *Layer) parseBaseHeader(baseHeader []byte, length int, minSize *int) (uint8, error) {
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

func (l *Layer) parseCSRC(csrcData []byte, csrcCount uint8) {
	l.csrcList = slice.AssignLen(l.csrcList[:0], int(csrcCount))
	for i := uint8(0); i < csrcCount; i++ {
		l.csrcList[i] = binary.BigEndian.Uint32(csrcData[:4])
		csrcData = csrcData[4:]
	}
	return
}

func (l *Layer) parseExtensionHeader(extensionHeader []byte, length int, minSize *int) (uint16, error) {
	l.extensionProfile = binary.BigEndian.Uint16(extensionHeader[:2])
	extensionLength := binary.BigEndian.Uint16(extensionHeader[2:4])

	// ensure packet size
	*minSize += int(extensionLength) * 4
	if length < *minSize {
		return extensionLength, PacketSizeNotEnoughError
	}
	return extensionLength, nil
}

func (l *Layer) parseExtensionData(extensionData []byte, extensionLength uint16) {
	l.extensionContents = slice.AssignLen(l.extensionContents[:0], int(extensionLength))
	for i := uint16(0); i < extensionLength; i++ {
		l.extensionContents[i] = binary.BigEndian.Uint32(extensionData[:4])
		extensionData = extensionData[4:]
	}
}

func (l *Layer) parsePadding(length int, minSize *int) error {
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

func (l *Layer) Unmarshal(data []byte) error {
	minSize := RTPHeaderMinSize
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
	l.content = cur[:length-minSize]
	l.extends = l.extends[:0]
	return nil
}

func (l *Layer) UnmarshalChunks(chunks [][]byte) error {
	if len(chunks) == 1 {
		return l.Unmarshal(chunks[0])
	}
	var length int
	var minSize = RTPHeaderMinSize
	for _, chunk := range chunks {
		length += len(chunk)
	}
	if length < minSize {
		return PacketSizeNotEnoughError
	}

	baseHeader, cur := SplitIntoBytesLinear(chunks, 12)
	csrcCount, err := l.parseBaseHeader(baseHeader, length, &minSize)
	if err != nil {
		return err
	}

	// parse csrc list
	csrcData, cur := SplitIntoBytesLinear(cur, uint(csrcCount*4))
	l.parseCSRC(csrcData, csrcCount)

	if l.HasExtension() {
		// parse extension header
		var extensionHeader, extensionData []byte
		extensionHeader, cur = SplitIntoBytesLinear(cur, 4)
		extensionLength, err := l.parseExtensionHeader(extensionHeader, length, &minSize)
		if err != nil {
			return err
		}

		// parse extension content
		extensionData, cur = SplitIntoBytesLinear(cur, uint(extensionLength*4))
		l.parseExtensionData(extensionData, extensionLength)
	}

	// parse padding
	if l.HasPadding() {
		l.paddingLength = ChunksLastByte(chunks)
		if err := l.parsePadding(length, &minSize); err != nil {
			return err
		}
	}

	// parse payload content
	payloadLength := length - minSize

	if payloadLength > 0 {
		contents := ChunksCutChunksTo(cur, uint(payloadLength))
		l.content = contents[0]
		l.extends = append(l.extends[:0], contents[1:]...)
	} else {
		l.content = nil
		l.extends = l.extends[:0]
	}

	return nil
}

const writeBufLen = 16

func (l *Layer) WriteTo(w io.Writer) (n int64, err error) {
	var buf [writeBufLen]byte

	// write base header
	buf[0], buf[1] = (l.CSRCCount()&0b00001111)|(l.first&0b11110000), l.second
	binary.BigEndian.PutUint16(buf[2:], l.sequenceNumber)
	binary.BigEndian.PutUint32(buf[4:], l.timestamp)
	binary.BigEndian.PutUint32(buf[8:], l.ssrc)
	if err = utils.Write(w, buf[:12], &n); err != nil {
		return
	}

	// write csrc list
	off := 0
	for _, csrc := range l.csrcList {
		binary.BigEndian.PutUint32(buf[off:], csrc)
		if off += 4; off == writeBufLen {
			if err = utils.Write(w, buf[:], &n); err != nil {
				return
			}
			off = 0
		}
	}
	if off > 0 {
		if err = utils.Write(w, buf[:off], &n); err != nil {
			return
		}
	}

	// write extension if necessary
	if l.HasExtension() {
		// write extension header
		extensionLength := l.ExtensionLength()
		binary.BigEndian.PutUint16(buf[:], l.extensionProfile)
		binary.BigEndian.PutUint16(buf[2:], extensionLength)
		off = 4

		// write extension content
		for i := uint16(0); i < extensionLength; i++ {
			binary.BigEndian.PutUint32(buf[off:], l.extensionContents[i])
			if off += 4; off == writeBufLen {
				if err = utils.Write(w, buf[:], &n); err != nil {
					return
				}
				off = 0
			}
		}
		if err = utils.Write(w, buf[:off], &n); err != nil {
			return
		}
	}

	// write payload content
	if err = utils.WriteTo(&l.PayloadContent, w, &n); err != nil {
		return
	}

	// write padding
	if l.HasPadding() && l.paddingLength > 0 {
		for i := 0; i < writeBufLen; i++ {
			buf[i] = 0
		}
		for remain := l.paddingLength; true; remain -= uint8(writeBufLen) {
			if remain <= uint8(writeBufLen) {
				buf[remain-1] = l.paddingLength
				if err = utils.Write(w, buf[:remain], &n); err != nil {
					return
				}
				break
			}
			if err = utils.Write(w, buf[:], &n); err != nil {
				return
			}
		}
	}

	return
}

func (l *Layer) Read(p []byte) (n int, err error) {
	w := utils.Writer{Buf: p}
	_n, _ := l.WriteTo(&w)
	return int(_n), io.EOF
}

func (l *Layer) Bytes() []byte {
	w := utils.Writer{Buf: make([]byte, l.Size())}
	n, _ := l.WriteTo(&w)
	return w.Buf[:n]
}

func (l *Layer) String() string {
	sb := strings.Builder{}
	sb.WriteString("RTP(Length=")
	sb.WriteString(strconv.FormatUint(uint64(l.Size()), 10))
	sb.WriteString(", PT=")
	sb.WriteString(strconv.FormatUint(uint64(l.PayloadType()), 10))
	sb.WriteString(", SSRC=0x")
	sb.WriteString(strconv.FormatUint(uint64(l.SSRC()), 16))
	if csrcCount := l.CSRCCount(); csrcCount > 0 {
		csrcList := l.CSRCList()
		sb.WriteString(", CSRC=[")
		for i := uint8(0); i < csrcCount; i++ {
			sb.WriteString("0x")
			sb.WriteString(strconv.FormatUint(uint64(csrcList[i]), 16))
			if i != csrcCount-1 {
				sb.WriteString(", ")
			}
		}
		sb.WriteString("]")
	}
	sb.WriteString(", Seq=")
	sb.WriteString(strconv.FormatUint(uint64(l.SequenceNumber()), 10))
	sb.WriteString(", Time=")
	sb.WriteString(strconv.FormatUint(uint64(l.Timestamp()), 10))
	sb.WriteString(", Payload=")
	sb.WriteString(strconv.FormatUint(uint64(l.PayloadLength()), 10))
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
