package server

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/creack/pty"
	"github.com/pion/webrtc/v2"
)

type dataChannelPipe struct {
	d *webrtc.DataChannel
}

func (pipe *dataChannelPipe) Write(p []byte) (n int, err error) {
	pipe.d.SendText(string(p))
	return len(p), nil
}

func NewWebRTCServer(config webrtc.Configuration) (pc *webrtc.PeerConnection, err error) {

	pc, err = webrtc.NewPeerConnection(config)
	if err != nil {
		return nil, fmt.Errorf("Failed to open peer connection: %q", err)
	}

	// Set the handler for ICE connection state
	// This will notify you when the peer has connected/disconnected
	pc.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		log.Printf("ICE Connection State has changed: %s\n", connectionState.String())
	})
	// Register data channel creation handling
	pc.OnDataChannel(func(d *webrtc.DataChannel) {
		if d.Label() == "signaling" {
			log.Printf("Got singalling channel")
			return
		}
		var cmd *exec.Cmd
		var ptmx *os.File
		var dcc *webrtc.DataChannel
		pipe := dataChannelPipe{d}
		cmdReady := make(chan bool, 1)
		d.OnOpen(func() {
			l := d.Label()
			log.Printf("New Data channel %q\n", l)
			c := strings.Split(l, " ")
			if c[0] == "CnTrL" {
			}
			dcc, err = pc.CreateDataChannel("c&c", &webrtc.DataChannelInit{})
			if err != nil || dcc == nil {
				log.Printf("Failed creating data channel: %v", err)
			}
			cmd = exec.Command(c[0], c[1:]...)
			ptmx, err = pty.Start(cmd)
			if err != nil {
				log.Panicf("Failed to attach a ptyi and start cmd: %v", err)
			}
			defer func() { _ = ptmx.Close() }() // Best effort.
			cmdReady <- true
			_, err = io.Copy(&pipe, ptmx)
			if err != nil {
				log.Printf("Failed to copy from pty: %v %v", err, cmd.ProcessState.String())
			}
			cmd.Process.Kill()
			d.Close()
			pc.Close()
		})
		d.OnClose(func() {
			// kill the command
			log.Println("Data channel closed")
		})
		d.OnMessage(func(msg webrtc.DataChannelMessage) {
			p := msg.Data
			<-cmdReady
			l, err := ptmx.Write(p)
			if err != nil {
				log.Printf("Stdin Write returned an error: %v %v", err, cmd.ProcessState.String())
			}
			if l != len(p) {
				log.Printf("stdin write wrote %d instead of %d bytes", l, len(p))
			}
			cmdReady <- true
		})
	})
	return pc, nil
}
