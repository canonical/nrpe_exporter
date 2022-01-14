package common

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

//define states
const (
	STATE_OK       = 0
	STATE_WARNING  = 1
	STATE_CRITICAL = 2
	STATE_UNKNOWN  = 3
)

//packet type
const (
	QUERY_PACKET    = 1
	RESPONSE_PACKET = 2
)

//packet version
const (
	NRPE_PACKET_VERSION_1 = 1
	NRPE_PACKET_VERSION_2 = 2
	NRPE_PACKET_VERSION_3 = 3 /* packet version identifier */
)

//max buffer size
const MAX_PACKETBUFFER_LENGTH = 1024

const HELLO_COMMAND = "version"

const PROGRAM_VERSION = "0.02"

type NrpePacket struct {
	PacketVersion int16
	PacketType    int16
	CRC32Value    uint32
	ResultCode    int16
	CommandBuffer [MAX_PACKETBUFFER_LENGTH]byte
	Trailer       int16
}

func CheckError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %s", err.Error())
		os.Exit(1)
	}
}

//todo return error as well
func ReceivePacket(conn net.Conn) (NrpePacket, error) {
	pkt_rcv := new(NrpePacket)
	if err := binary.Read(conn, binary.BigEndian, pkt_rcv); err != nil {
		return *pkt_rcv, err
	}
	return *pkt_rcv, nil
}

func SendPacket(conn net.Conn, pkt_send NrpePacket) error {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.BigEndian, &pkt_send); err != nil {
		fmt.Println(err)
	}
	if _, err := conn.Write([]byte(buf.Bytes())); err != nil {
		return err
	}
	return nil
}

func PrepareToSend(cmd string, pkt_type int16) NrpePacket {
	pkt_send, _ := MakeNrpePacket()
	if pkt_type == RESPONSE_PACKET { //its a response
		pkt_send.PacketType = RESPONSE_PACKET
		if cmd == HELLO_COMMAND {
			copy(pkt_send.CommandBuffer[:], PROGRAM_VERSION)
			pkt_send.ResultCode = STATE_OK
		}
	} else { // Query Packet
		pkt_send.ResultCode = STATE_OK
		pkt_send.PacketType = QUERY_PACKET
		copy(pkt_send.CommandBuffer[:], cmd)
	}
	pkt_send.CRC32Value, _ = DoCRC32(&pkt_send)
	return pkt_send
}

func MakeNrpePacket() (NrpePacket, error) {
	pkt := new(NrpePacket)
	buf := make([]byte, len(pkt.Encode()))

	char := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	rand.Seed(time.Now().UTC().UnixNano())
	for i := 0; i < len(buf); i++ {
		buf[i] = char[rand.Intn(len(char)-1)]
	}
	if err := binary.Read(bytes.NewReader(buf), binary.BigEndian, pkt); err != nil {
		return *pkt, err
	}

	pkt.PacketVersion = NRPE_PACKET_VERSION_2
	pkt.CRC32Value = 0
	pkt.ResultCode = STATE_UNKNOWN
	for i := range pkt.CommandBuffer {
		pkt.CommandBuffer[i] = '\x00'
	}
	return *pkt, nil
}

func DoCRC32(pkt *NrpePacket) (uint32, error) {
	pkt.CRC32Value = 0
	pktbytes := pkt.Encode()

	return crc32.ChecksumIEEE(pktbytes), nil
}

func (pkt *NrpePacket) Encode() []byte {
	writer := new(bytes.Buffer)
	binary.Write(writer, binary.BigEndian, pkt.PacketVersion)
	binary.Write(writer, binary.BigEndian, pkt.PacketType)
	binary.Write(writer, binary.BigEndian, pkt.CRC32Value)
	binary.Write(writer, binary.BigEndian, pkt.ResultCode)
	writer.Write([]byte(pkt.CommandBuffer[:]))
	binary.Write(writer, binary.BigEndian, pkt.Trailer)
	return writer.Bytes()
}

// count the numbers of bytes until 0 is found
func GetLen(b []byte) int {
	return bytes.Index(b, []byte{0})
}

func ExecuteCommand(cmd_in string) (int16, []byte) {
	parts := strings.Fields(cmd_in)
	head := parts[0]
	parts = parts[1:]
	cmd := exec.Command(head, parts...)
	cmd_stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		return int16(2), nil
	}
	stdout_reader := bufio.NewReader(cmd_stdout)
	read_line, _, _ := stdout_reader.ReadLine()
	result := cmd.Wait()
	status := 0
	if result != nil {
		status = result.(*exec.ExitError).ProcessState.Sys().(syscall.WaitStatus).ExitStatus()
	}
	return int16(status), read_line
}
