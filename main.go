package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
)

const (
	udpPort       = 8080 // Change this to the desired UDP port
	host          = "0.0.0.0"
	slinChunkSize = 320
)

type UdpWrapper struct {
	conn *net.UDPConn
	addr *net.UDPAddr
}

func NewUdpWrapper(addr *net.UDPAddr, conn *net.UDPConn) io.Writer {
	return &UdpWrapper{
		addr: addr,
		conn: conn,
	}
}

func (u *UdpWrapper) Write(data []byte) (n int, err error) {
	n, err = u.conn.WriteToUDP(data, u.addr)
	if err != nil {
		fmt.Printf("Error writing to UDP: %v\n", err)
		return
	}
	return len(data), nil
}

func LoadAudioFile(fileName string) ([]byte, error) {
	var err error
	audioResponse, err := os.ReadFile(fileName)
	if err != nil {
		log.Println("failed to read audio file:", err)
		return audioResponse, err
	}
	return audioResponse, nil
}

func sendAudio(w io.Writer, data []byte) error {

	// NOTE: MTU (Max Transmission Unit), empirical tests have revealed that 160 is the value of Asterisk
	//
	// MTU: 160
	// PT: PCMA (G711/alaw)
	// SSRC: TODO read RFC to implement proper number generator
	// PAYLOADER: codecs.G711 (alaw/ulaw)
	// SEQUENCER: choose any of them
	// CLOCK RATE: 8000 (ulaw will allways be this anyway)
	packetizer := rtp.NewPacketizer(172, rtp.PayloadTypePCMA, 0x554c80c9, &codecs.G711Payloader{}, rtp.NewFixedSequencer(uint16(5000)), 8000)
	pkts := packetizer.Packetize(data, 1)

	t := time.NewTicker(20 * time.Millisecond)
	defer t.Stop()
	i := 0
	pkts[0].Timestamp = 0
	var accumulator uint32 = 0

	for range t.C {
		if i >= len(pkts) {
			return nil
		}
		pkts[i].Timestamp = accumulator // NOTE: Asterisk needs Timestamp to be the number of bytes sent, thats why the accumulator is here
		payload, err := pkts[i].Marshal()
		if err != nil {
			log.Println("packet to byte payload conversion got error", err)
			i++
			continue
		}
		if _, err := w.Write(payload); err != nil {
			return errors.New("failed to write chunk to audiosocket")
		}
		accumulator += uint32(len(pkts[i].Payload))
		i++
	}
	return errors.New("ticker unexpectedly stopped")
}

func handleSend(w io.Writer, addr string) {
	audio, err := LoadAudioFile("./test.alaw")
	if err != nil {
		log.Println("Got error while loading test.alaw")
		return
	}

	if err := sendAudio(w, audio); err != nil {
		fmt.Printf("Error writing to UDP: %v\n", err)
	}
	log.Printf("Sent Audio back to %s", addr)
}

func main() {
	running := true
	udpAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", host, udpPort))
	if err != nil {
		fmt.Printf("Error resolving UDP address for RTP server: %v\n", err)
		os.Exit(1)
	}

	conn, err := net.ListenUDP("udp4", udpAddr)
	if err != nil {
		fmt.Printf("Error creating UDP connection: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	log.Printf("RTP server is listening on UDP port %d...\n", udpPort)

	buffer := make([]byte, 4096) // Adjust buffer size as needed
	var addr *net.UDPAddr

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	go func() {
		for sig := range stop {
			log.Printf("Stoping rtcp server now, %s", sig.String())
			conn.Close()
			running = false
		}
	}()

	fileData := make([]byte, 4096)
	defer func() {
		err := os.Truncate("./udp_test.slin", int64(len(fileData)))
		if err != nil {
			log.Println("could not truncate audio file")
			return
		}
		file, err := os.OpenFile("./udp_test.slin", os.O_CREATE|os.O_RDWR, 0755)
		if err != nil {
			log.Println("Something went wrong while opening file", err)
			return
		}
		defer file.Close()
		n, err := file.Write(fileData)
		if err != nil {
			log.Println("could not write data to file for some reason")
			return
		}
		log.Printf("%d writen", n)
	}()

	n, addr, err := conn.ReadFromUDP(buffer)
	if err != nil {
		fmt.Printf("Error reading from UDP: %v\n", err)
		return
	}
	udpWrapper := NewUdpWrapper(addr, conn)
	go handleSend(udpWrapper, addr.String())

	for running {
		pkt := rtp.Packet{}
		err = pkt.Unmarshal(buffer[:n])
		if err != nil {
			log.Printf("Warn: could not Unmarshal rtp Packet")
		}
		fileData = append(fileData, pkt.Payload...)

		n = 0
		for n == 0 {
			n, _, err = conn.ReadFromUDP(buffer)
			if err != nil {
				fmt.Printf("Error reading from UDP: %v\n", err)
				return
			}
		}
	}
}
