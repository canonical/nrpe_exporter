/*
Package nrpe implements NRPE client/server library for go.

It supports plain mode and fully compatible with standard nrpe library.
*/
package nrpe

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"time"
	"unsafe"
)

var crc32Table []uint32

var randSource *rand.Rand

const (
	maxPacketDataLength = 1024
	packetLength        = maxPacketDataLength + 12
)

const (
	queryPacketType    = 1
	responsePacketType = 2
)

const (
	//currently supporting latest version2 protocol
	nrpePacketVersion2 = 2
)

// Result status codes
const (
	StatusOK       = 0
	StatusWarning  = 1
	StatusCritical = 2
	StatusUnknown  = 3
)

// CommandStatus represents result status code
type CommandStatus int

// CommandResult holds information returned from nrpe server
type CommandResult struct {
	StatusLine string
	StatusCode CommandStatus
}

type packet struct {
	packetVersion []byte
	packetType    []byte
	crc32         []byte
	statusCode    []byte
	padding       []byte
	data          []byte

	all []byte
}

// Initialization of crc32Table and randSource
func init() {
	var crc, poly, i, j uint32

	crc32Table = make([]uint32, 256)

	poly = uint32(0xEDB88320)

	for i = 0; i < 256; i++ {
		crc = i

		for j = 8; j > 0; j-- {
			if (crc & 1) != 0 {
				crc = (crc >> 1) ^ poly
			} else {
				crc >>= 1
			}
		}

		crc32Table[i] = crc
	}

	randSource = rand.New(rand.NewSource(time.Now().UnixNano()))
}

//Builds crc32 from the given input
func crc32(in []byte) uint32 {
	var crc uint32

	crc = uint32(0xFFFFFFFF)

	for _, c := range in {
		crc = ((crc >> 8) & uint32(0x00FFFFFF)) ^ crc32Table[(crc^uint32(c))&0xFF]
	}

	return (crc ^ uint32(0xFFFFFFFF))
}

//extra randomization for encryption
func randomizeBuffer(in []byte) {
	n := len(in) >> 2

	for i := 0; i < n; i++ {
		r := randSource.Uint32()

		copy(in[i<<2:(i+1)<<2], (*[4]byte)(unsafe.Pointer(&r))[:])
	}

	if len(in)%4 != 0 {
		r := randSource.Uint32()

		copy(in[n<<2:], (*[4]byte)(unsafe.Pointer(&r))[:len(in)-(n<<2)])
	}
}

// Command represents command name and argument list
type Command struct {
	Name string
	Args []string
}

// NewCommand creates Command object with the given name and optional argument list
func NewCommand(name string, args ...string) Command {
	return Command{
		Name: name,
		Args: args,
	}
}

// toStatusLine convers Command content to single status line string
func (c Command) toStatusLine() string {
	if c.Args != nil && len(c.Args) > 0 {
		args := strings.Join(c.Args, "!")
		return c.Name + "!" + args
	}

	return c.Name
}

func createPacket() *packet {
	var p packet
	p.all = make([]byte, packetLength)

	p.packetVersion = p.all[0:2]
	p.packetType = p.all[2:4]
	p.crc32 = p.all[4:8]
	p.statusCode = p.all[8:10]
	p.data = p.all[10 : packetLength-2]

	return &p
}

// verifyPacket checks packetType and crc32
func verifyPacket(responsePacket *packet, packetType uint16) error {
	be := binary.BigEndian

	rpt := be.Uint16(responsePacket.packetType)
	if rpt != packetType {
		return fmt.Errorf(
			"nrpe: Error response packet type, got: %d, expected: %d",
			rpt, packetType)
	}

	crc := be.Uint32(responsePacket.crc32)

	be.PutUint32(responsePacket.crc32, 0)

	if crc != crc32(responsePacket.all) {
		return fmt.Errorf("nrpe: Response crc didn't match")
	}
	return nil
}

// readCommandResult creates CommandResult object from packet
func readCommandResult(p *packet) (*CommandResult, error) {
	var result CommandResult
	be := binary.BigEndian

	pos := bytes.IndexByte(p.data, 0)

	if pos != -1 {
		result.StatusLine = string(p.data[:pos])
	}

	code := be.Uint16(p.statusCode)

	switch code {
	case StatusOK, StatusWarning, StatusCritical, StatusUnknown:
		result.StatusCode = CommandStatus(code)
	default:
		return nil, fmt.Errorf("nrpe: Unknown status code %d", code)
	}

	return &result, nil
}

// buildPacket creates packet structure
func buildPacket(packetType uint16, statusCode uint16, statusLine []byte) *packet {
	be := binary.BigEndian

	p := createPacket()

	randomizeBuffer(p.all)

	be.PutUint16(p.packetVersion, nrpePacketVersion2)
	be.PutUint16(p.packetType, packetType)
	be.PutUint32(p.crc32, 0)
	be.PutUint16(p.statusCode, statusCode)

	length := len(statusLine)

	if length >= maxPacketDataLength {
		length = maxPacketDataLength - 1
	}
	copy(p.data, statusLine[:length])
	p.data[length] = 0

	be.PutUint32(p.crc32, crc32(p.all))

	return p
}

// writePacket writes packet content to connection
func writePacket(conn net.Conn, timeout time.Duration, p *packet) error {
	if timeout > 0 {
		conn.SetWriteDeadline(time.Now().Add(timeout))
	}

	l, err := conn.Write(p.all)

	if err != nil {
		return err
	}

	if l != len(p.all) {
		return fmt.Errorf("nrpe: error while writing")
	}

	return nil
}

// readPacket reads from connection to packet
func readPacket(conn net.Conn, timeout time.Duration, p *packet) error {
	if timeout > 0 {
		conn.SetReadDeadline(time.Now().Add(timeout))
	}

	l, err := conn.Read(p.all)

	if err != nil {
		return err
	}

	if l != len(p.all) {
		return fmt.Errorf("nrpe: error while reading")
	}

	return nil
}

// Run specified command
func Run(conn net.Conn, command Command, isSSL bool,
	timeout time.Duration) (*CommandResult, error) {

	var err error

	// setup ssl connection
	if isSSL {
		return nil, fmt.Errorf("SSL not implemented in this fork!")
	}

	statusLine := command.toStatusLine()

	if len(statusLine) >= maxPacketDataLength {
		return nil, fmt.Errorf("nrpe: Command is too long: got %d, max allowed %d",
			len(statusLine), maxPacketDataLength-1)
	}

	request := buildPacket(queryPacketType, 0, []byte(statusLine))

	if err = writePacket(conn, timeout, request); err != nil {
		return nil, err
	}

	response := createPacket()

	if err = readPacket(conn, timeout, response); err != nil {
		return nil, err
	}

	if err = verifyPacket(response, responsePacketType); err != nil {
		return nil, err
	}

	var result *CommandResult

	if result, err = readCommandResult(response); err != nil {
		return nil, err
	}

	return result, nil
}

// ServeOne function will handle one request. After receiving request
// it will call handler callback function and the result of callback
// will be sent to requester.
func ServeOne(conn net.Conn, handler func(Command) (*CommandResult, error),
	isSSL bool, timeout time.Duration) error {

	var err error

	// setup ssl
	if isSSL {
		return fmt.Errorf("SSL not implemented in this fork!")
	}

	request := createPacket()

	if err = readPacket(conn, timeout, request); err != nil {
		return err
	}

	if err = verifyPacket(request, queryPacketType); err != nil {
		return err
	}

	var pos = bytes.IndexByte(request.data, 0)

	if pos == -1 {
		return fmt.Errorf("nrpe: invalid request")
	}

	data := strings.Split(string(request.data[:pos]), "!")

	result, err := handler(NewCommand(data[0], data[1:]...))

	if err != nil {
		return err
	}

	response := buildPacket(responsePacketType,
		uint16(result.StatusCode), []byte(result.StatusLine))

	if err = writePacket(conn, timeout, response); err != nil {
		return err
	}

	return nil
}
