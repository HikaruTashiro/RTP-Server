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

	//"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
)

const (
	udpPort       = 8080 // Change this to the desired UDP port
	host          = "192.168.0.10"
	slinChunkSize = 320

	// AstHost    = "192.168.0.10"
	// AstudpPort = 8080 // Find how can i send audio back to Asterisk
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
	log.Printf("Sent Audio back to %s", u.addr.String())
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

// FIXME: only receives beggining and stops, determine what is happening to audio (Stuck on Asterisk or Sip Phone?)
func sendAudio(w io.Writer, data []byte) error {

	// NOTE: MTU (Max Transmission Unit), empirical tests have revealed that 160 is the value of Asterisk
	//
	// MTU: 160
	// PT: PCMA (G711/alaw)
	// SSRC: Some number (don't really know what is the best approach)
	// PAYLOADER: codecs.G711 (alaw/ulaw)
	// SEQUENCER: choose any of them
	// CLOCK RATE: 8000 (ulaw will allways be this anyway)
	packetizer := rtp.NewPacketizer(172, rtp.PayloadTypePCMA, 0x554c80c9, &codecs.G711Payloader{}, rtp.NewFixedSequencer(uint16(5000)), 8000)
	pkts := packetizer.Packetize(data, 1)

	t := time.NewTicker(20 * time.Millisecond)
	defer t.Stop()
	i := 0

	for range t.C {
		if i >= len(pkts) {
			return nil
		}
		payload, err := pkts[i].Marshal()
		if err != nil {
			log.Println("packet to byte payload conversion got error", err)
			i++
			continue
		}
		if _, err := w.Write(payload); err != nil {
			return errors.New("failed to write chunk to audiosocket")
		}
		i++
	}
	return errors.New("ticker unexpectedly stopped")
}

func handleSend(w io.Writer, addr string) {
	audio, err := LoadAudioFile("./nice_song.raw")
	if err != nil {
		log.Println("Got error while loading nice_song")
		return
	}

	if err := sendAudio(w, audio); err != nil {
		fmt.Printf("Error writing to UDP: %v\n", err)
	}
	log.Printf("Sent Audio back to %s", addr)
}

func main() {
	running := true
	// Resolve the UDP address
	udpAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", host, udpPort))
	if err != nil {
		fmt.Printf("Error resolving UDP address for RTP server: %v\n", err)
		os.Exit(1)
	}

	// Create a UDP connection
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
		// RTP
		pkt := rtp.Packet{}
		err = pkt.Unmarshal(buffer[:n])
		if err != nil {
			log.Printf("Warn: could not Unmarshal rtp Packet")
		}
		fileData = append(fileData, pkt.Payload...)
		//_, _ = conn.WriteToUDP(buffer[:n], addr)
		//log.Printf("payload:\n%s\n", pkt.String())
		log.Printf("Received Audio from %s", addr.String())

		n = 0
		for n == 0 {
			n, addr, err = conn.ReadFromUDP(buffer)
			if err != nil {
				fmt.Printf("Error reading from UDP: %v\n", err)
				return
			}
		}
	}
}
